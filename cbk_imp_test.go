package cbk_s1mpl3

import (
	"cbk-s1mpl3/util"
	"fmt"
	log "github.com/sirupsen/logrus"
	"testing"
	"time"
)

func init() {
	log.SetFormatter(&log.TextFormatter{
		//DisableColors: true,
		FullTimestamp:   true,
		TimestampFormat: time.StampMilli,
	})
	lvl, _ := log.ParseLevel("debug")
	log.SetLevel(lvl)
}

const HOST_PREFIX = "http://www.abc.com"
const API_PREFIX = "/fake-api"

var (
	// 是否熔断过
	HasCbk = false
)

func TestCircuitBreakerImp(t *testing.T) {
	log.Infof("Test for cbk: %s", HOST_PREFIX+API_PREFIX)

	cbk := &CircuitBreakerImp{}
	cbk.apiMap = make(map[string]*apiSnapShop)
	// 控制时间窗口，15秒一轮, 重置api错误率
	cbk.roundInterval = util.ToDuration(15 * time.Second)
	// 熔断之后，5秒不出现错误再恢复
	cbk.recoverInterval = util.ToDuration(5 * time.Second)
	cbk.minCheck = 5
	cbk.cbkErrRate = 0.5
	StartJob(cbk)
}

func StartJob(cbk *CircuitBreakerImp) {
	for {
		// 每秒发1次失败
		ReqForTest(cbk, 0)
		time.Sleep(time.Second * 1)
	}
}

func reportStatus(cbk *CircuitBreakerImp) {
	for {
		log.Debug("Report for cbk status...")
		for k, v := range cbk.apiMap {
			log.Debugf("Cbk map status: API: %v, isPaused: %v, errCount: %v,"+
				" total: %v, accessLast: %v, rountLast: %v", k, v.isPaused,
				v.errCount, v.totalCount, v.accessLast, v.roundLast)
		}
		time.Sleep(3 * time.Second)
	}
}

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

func TestSome(t *testing.T) {
	fmt.Print("just for fun")
}
