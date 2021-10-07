package cbk_s1mpl3

// Cbk接口
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

type Error struct {
	error
	Msg string
}

// 熔断器Error
func (cerr Error) Error() string {
	if cerr.Msg != "" {
		return cerr.Msg
	}
	return "CircuitBreaker is break"
}
