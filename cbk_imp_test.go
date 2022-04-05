package cbk_s1mpl3

import (
	"cbk-s1mpl3/util"
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

const HOST_PREFIX = "localhost:8888"
const MOCK_API = "/fake-api"

var (
	NeetCoolDown = false
)

func TestCircuitBreakerImp(t *testing.T) {
	log.Infof("Test for cbk: %s", HOST_PREFIX+MOCK_API)

	cbk := &CircuitBreakerImp{}
	cbk.apiMap = make(map[string]*apiSnapShop)
	// reset api metric when round end per 15s
	cbk.roundInterval = util.ToDuration(15 * time.Second)
	// allow to access when cbk triggered in a round per 5s
	cbk.recoverInterval = util.ToDuration(5 * time.Second)
	cbk.minCheck = 5
	cbk.cbkErrRate = 0.5
	StartMock(cbk)
}

func StartMock(cbk *CircuitBreakerImp) {
	for {
		// mock failed for every second
		ticker := time.NewTicker(time.Second)
		select {
		case <-ticker.C:
			ReqForTest(cbk, 0)
		}
	}
}

func ReqForTest(cbk *CircuitBreakerImp, turn int) {
	//log.Infof("Ready to reqForTest: %s, turn-id-%v", HOST_PREFIX+MOCK_API, turn)

	if !cbk.CanAccess(MOCK_API) {
		log.Warnf("Api: %v is break, wait for next round or success for one...", MOCK_API)
		NeetCoolDown = true
		return
	} else {
		//log.Infof("Request access allow: %s", HOST_PREFIX+MOCK_API)
		// after
		if NeetCoolDown && turn == 0 {
			NeetCoolDown = false
			turn = 1
			log.Warnf("Transfer fail to success: %s, turn-id-%v", HOST_PREFIX+MOCK_API, turn)
		}
	}

	if turn == 0 {
		log.Errorf("# Meet failed ReqForTest: %s", HOST_PREFIX+MOCK_API)
		cbk.Failed(MOCK_API)
	} else {
		log.Infof("# Meet success ReqForTest: %s", HOST_PREFIX+MOCK_API)
		cbk.Succeed(MOCK_API)
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
