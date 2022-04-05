# cbk-s1mpl3
熔断器的Go语言版本简单实现：让相同API到达一定错误率时进行快速失败。

## English Readme 
[English Version](README.md)

## 熔断器
在一定的时间段内，达到一定的失败率，则变为熔断。
- 正常 --> 熔断状态:
    达到请求次数 && 达到失败率
- 熔断 --> 正常状态:
    请求成功
    
## 详细实现:
个人掘金专栏: [聊一聊Go实现的接口熔断器: Circuit Breaker](https://juejin.cn/post/6999173910851747853)

## 相关概念
- **动态调整**  
熔断器的核心功能在于“动态调整”，就是说在一定时间窗口内，当接口出现错误的次数，或者说比率达到危险值，则熔断器必须**hold**住请求，快速返回(失败)，告知上游该接口**需要冷静一下**，主要有下面几个参数：
  - **阈值**：包括两个指标：最少统计次数和错误占比。最少统计次数用于捕捉高频请求，即请求量至少要大于多少，熔断器才会开始关心错误率，否则会导致其他低频接口发生少量失败就触发熔断。
  - **重置周期**：重置周期就是熔断器调度的时间窗口，可以类比于限流算法中的单位时间，即经过多长时间，熔断器周期重置接口总次数和错误计数。
- **恢复周期**：是指在发生熔断之后，经过多长的空闲时期给予接口重试权限，尝试让其纠正、恢复到正常状况。  
这个和重置周期的主要区别是：熔断器重置计数之后，++接口必须再次达到错误率才会触发熔断，而恢复周期仅仅是允许当前熔断的接口请求，其错误率还是很高，如果再次尝试请求还是没恢复，则保持熔断状态，等待下一次恢复周期，或者重置周期。++ 因此**恢复周期**<**重置周期**。  

关于状态转移，这张示例图比较形象：  
![熔断器状态转换](https://pixelpig-1253685321.cos.ap-guangzhou.myqcloud.com/blog/cbk/cbk.png)
- **AOP切入**  
熟悉熔断器的基本用途之后，再来分析其组件在项目中的位置。
相信大部分同学应该了解面向切片的概念，很多通用组件，像流量限制、熔断保护都是面向切片的模块，通常用于采集并上报流量状况，按照特定规则保护下游。最常见的就是把它们放在网关层。  
**如下图：作为AOP横切面分别在请求发起和返回处做指标采集**  
![熔断器切片位置](https://pixelpig-1253685321.cos.ap-guangzhou.myqcloud.com/blog/cbk/aop.png)
## 代码实现
了解了其核心功能之后，接下来看下代码实现部分。
### 接口定义
基于熔断器的特性，先定义熔断器接口```CircuitBreaker```，其中key指熔断器采集的API，可以是链路上面的请求标识，用于区分不同请求。
```go
type CircuitBreaker interface {
    // 当前key(api接口)是否可访问
    CanAccess(key string) bool
    // 标记访问, 并且如果当前key失败率到达阈值, 则开启熔断
    Failed(key string)
    // 标记访问, 并且如果当前key已熔断, 则恢复
    Succeed(key string)
    // 返回当前key的熔断状态
    IsBreak(key string) bool
    // 返回所有key(api接口)当前状态, 用于查询汇报
    Status() interface{}
}
```
### 接口实现
基于上面约定的几个函数特性，创建```CircuitBreakerImp```作为实现体
```go
// 熔断器实现体
type CircuitBreakerImp struct {
    lock            sync.RWMutex
    apiMap          map[string]*apiSnapShop // api全局map，key为API标志
    minCheck        int64                   // 接口熔断开启下限次数
    cbkErrRate      float64                 // 接口熔断开启比值
    recoverInterval time.Duration           // 熔断恢复区间
    roundInterval   time.Duration           // 计数重置区间
}
```
熔断器实现体的```apiMap```主要存放的是采集API的列表，用于表示当前阶段所收集各个API的总体快照，
```go
type apiSnapShop struct {
    isPaused   bool  // api是否熔断
    errCount   int64 // api在周期内失败次数
    totalCount int64 // api在周期内总次数

    accessLast int64 // api最近一次访问时间
    roundLast  int64 // 熔断器周期时间
}
```
----
**熔断状况获取**，基于每个失败过的请求，会将其请求快照存储在当前```CircuitBreakerImp```实例的apiMap中，所以```IsBreak()```接口的实现相对简单，直接返回其状态即可：
```go
func (c *CircuitBreakerImp) IsBreak(key string) bool {
    c.lock.RLock()
    defer c.lock.RUnlock()

    // 查找当前key(API)是否达到熔断
    if api, ok := c.apiMap[key]; ok {
        return api.isPaused
    }
    return false
}
```
----
**访问更新**，每次网关接收请求时候，会在请求发起处和请求返回处上报接口访问次数、返回成功与否的信息给熔断器，如上面的AOP图片所示，因此实现```accessed()```函数用于在每次请求API进行上报。  
此外，前面提及到熔断器有个**重置周期**，即经过多少时间接口再次访问会重置其快照计数，因此在每次```access```处，需要多加一个判断，用于重置API计数。  
```go
// accessed 记录访问
func (c *CircuitBreakerImp) accessed(api *apiSnapShop) {
    /*
        判断是否大于周期时间
        - 是: 重置计数
        - 否: 更新计数
    */
    now := time.Now().UnixNano()
    if util.Abs64(now-api.roundLast) > int64(c.roundInterval) {
        if api.roundLast != 0 {
            // 首次不打log
            log.Warnf("# Trigger 熔断器窗口关闭，重置API计数!")
        }
        api.errCount = 0
        api.totalCount = 0
        api.roundLast = now
    }
    api.totalCount++
    api.accessLast = now
}
```
----
**上报成功访问**，在网关接收到返回成功的响应码之后，会记录其成功，另外有个细节，就是为什么上面的```accessed()```函数不需要添加互斥锁，其实是需要的，只不过在```accessed()```函数的上一层调用处加上，即后续```Successd()```和```Failed()```函数体中。  
先来看下```Successd()```的实现，**如果当前api记录在熔断器中(曾经失败过)，则成功之后关闭熔断给予请求**，即使api再次失败，也会根据请求错误率重新分配熔断状态。
```go
/*
    Succeed 记录成功
    只更新api列表已有的,
    记录访问, 并判断是否熔断:
    - 是, 取消熔断状态
*/
func (c *CircuitBreakerImp) Succeed(key string) {
    c.lock.Lock()
    c.lock.Unlock()

    if api, ok := c.apiMap[key]; ok {
        c.accessed(api)
        if api.isPaused {
            log.Warnf("# Trigger API: %v 请求成功，关闭熔断状态.", key)
            api.isPaused = false
        }
    }
}
```
----
**上报失败访问**，统计失败的逻辑也相对简单，对于首次失败的接口将其加入健康检测map，对于已经记录的接口，需要判断其错误率进行及时更新，此外触发熔断的前提是请求量还要达到一定的阈值。
```go
/*
    Failed 记录失败访问
    api列表查找,
        - 已有:
            - 记录访问/错误次数
            - 是否失败占比到达阈值? 是, 则标记置为熔断
        - 未找到:
            更新至api列表: 记录访问/错误次数
*/
func (c *CircuitBreakerImp) Failed(key string) {
    c.lock.Lock()
    defer c.lock.Unlock()

    if api, ok := c.apiMap[key]; ok {
        c.accessed(api)
        api.errCount++

        errRate := float64(api.errCount) / float64(api.totalCount)
        // 请求数量达到阈值 && 错误率高于熔断界限
        if api.totalCount > c.minCheck && errRate > c.cbkErrRate {
            log.Warnf("# Trigger 达到错误率, 开启熔断！: %v, total: %v, "+
                "errRate: %.3f.", key, api.totalCount, errRate)
            api.isPaused = true
        }
    } else {
        api := &apiSnapShop{}
        c.accessed(api)
        api.errCount++
        // 写入全局map
        c.apiMap[key] = api
    }
}
```
----
**访问权限查询**，基于上面实现的函数熔断器的核心功能就具备了，之后还需要为网关层提供一个获取当前接口的熔断状况，下面是为调用方提供的**访问查询**。  

这里有个疑问，上面不是已经实现了```IsBreak()```函数吗，为何还需要```CanAccess()```函数?  
那是因为，```IsBreak()```返回仅仅是接口的熔断状态，但是别忘了熔断阶段里面有一个“半关闭状态”，即如果熔断时间度过恢复期(冷静期)，则可以放开访问权限，所以这部分逻辑是放在```CanAccess()```里面处理的，来看下代码。  
其中，入参key代表网关层要访问的接口标识，如```/get-hello```，函数返回值表示该接口的可访问状态。  
> 在熔断周期内，度过恢复期(冷静期)，则可以放开访问，或者调用了```Successd()```熔断状态变为临时恢复，否则保持熔断状态，接口会被限制，快速失败的核心代码就在这里。
```go
/*
 CanAccess 判断api是否可访问
 debug:
 func (c *CircuitBreakerImp) CanAccess(key string, reqType int) bool {
*/
func (c *CircuitBreakerImp) CanAccess(key string) bool {
	/*
        判断当前api的isPaused状态
        - 未熔断, 返回true
        - 已熔断, 当前时间与恢复期比较
            - 大于恢复期, 返回true
            - 小于恢复期, 返回false
	*/
    c.lock.RLock()
    defer c.lock.RUnlock()
    log.Debugf("# Cbk check accessable for api id-%v key", reqType)
    // 从api全局map查找
    if api, ok := c.apiMap[key]; ok {
        log.Debugf("# Cbk detail for api id-%v key, total: %v, "+
            "errCount: %v, paused: %v", reqType, api.totalCount,
            api.errCount, api.isPaused)
            
        if api.isPaused {
            // 判断是否进入恢复期
            latency := util.Abs64(time.Now().UnixNano() - api.accessLast)
            if latency < int64(c.recoverInterval) {
                // 在恢复期之内, 快速失败，保持熔断
                return false
            }
            // 度过恢复期
            log.Warnf("# Trigger: 熔断器度过恢复期: %v, key: %v!", c.recoverInterval, key)
        }
    }
    // 给予临时恢复
    return true
}
```

## 单元测试
基于上面的实现，接下来编写测试代码进行case覆盖，演示并且循环滚动保持这两个过程：
- **失败->持续失败->熔断开启->经过冷却时间**  
- **进入恢复期->继续访问**  

### 模拟虚假请求
```go
const API_PREFIX = "/fake-api"
var (
    // 是否熔断过
    HasCbk = false
)

func StartJob(cbk *CircuitBreakerImp) {
    for {
        // 每秒发1次失败, 参数0代表failed, 1代表成功
        ReqForTest(cbk, 0)
        time.Sleep(time.Second * 1)
    }
}

// 构建请求调度，熔断恢复其让其成功1次
func ReqForTest(cbk *CircuitBreakerImp, req int) {
    // mock failed case
    mockAPI := API_PREFIX //+ strconv.Itoa(req)
    //log.Infof("Ready to reqForTest: %s, req-id-%v", HOST_PREFIX+mockAPI, req)

    if !cbk.CanAccess(mockAPI, req) {
        log.Warnf("Api: %v is break, req-id-%v, wait for next round or success for one...", mockAPI, req)
        HasCbk = true
        return
    } else {
        log.Infof("Request can access: %s, req-id-%v", HOST_PREFIX+mockAPI, req)
        // 度过恢复期, 熔断恢复之后, 跳过错误让其成功
        if HasCbk && req == 0 {
            HasCbk = false
            req = 1
            log.Warnf("Transfer fail to success: %s, req-id-%v", HOST_PREFIX+mockAPI, req)
        }
    }

    if req == 0 {
        log.Errorf("# Meet failed ReqForTest: %s", HOST_PREFIX+mockAPI)
        cbk.Failed(mockAPI)
    } else {
        log.Infof("# Meet success ReqForTest: %s", HOST_PREFIX+mockAPI)
        cbk.Succeed(mockAPI)
    }
}
```
### 初始化熔断器
初始化熔断器并分配相关参数
```go
cbk := &CircuitBreakerImp{}
cbk.apiMap = make(map[string]*apiSnapShop)
// 控制时间窗口，15秒一轮, 重置api错误率
cbk.roundInterval = util.ToDuration(15 * time.Second)
// 熔断之后，5秒不出现错误再恢复
cbk.recoverInterval = util.ToDuration(5 * time.Second)
// 请求到达5次以上才进行熔断检测
cbk.minCheck = 5
// 错误率到达 50% 开启熔断
cbk.cbkErrRate = 0.5
```

### 程序输出解析
**可以看到程序输出如下，符合预期**
![熔断状态](https://pixelpig-1253685321.cos.ap-guangzhou.myqcloud.com/blog/cbk/cbk_status_0.png)
1. 在接口前5次请求虽然失败率是100%，但是请求量没上去因此熔断器没有触发直到
2. 后续熔断条件满足，开启熔断
3. 熔断状态持续经过5秒后，转为空闲期，api转变为可访问，再次访问接口，错误率依旧满足，则继续恢复熔断状态。
----
![熔断重置计数](https://pixelpig-1253685321.cos.ap-guangzhou.myqcloud.com/blog/cbk/cbk_status_1.png)
经过重置周期，API计数重置，则又回到程序启动处那种状态，接口恢复**持续访问**，直到到达错误率开启熔断。

## 参考资料
**Circuit Breaker Pattern**  
https://docs.microsoft.com/en-us/previous-versions/msp-n-p/dn589784(v=pandp.10)  
**Sony实现的gobreaker**  
https://github.com/sony/gobreaker  
**微服务架构中的熔断器设计与实现**  
https://mp.weixin.qq.com/s/DGRnUhyv6SS_E36ZQKGpPA