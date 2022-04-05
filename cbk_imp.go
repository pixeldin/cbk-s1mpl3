package cbk_s1mpl3

import (
	"cbk-s1mpl3/util"
	log "github.com/sirupsen/logrus"
	"sync"
	"time"
)

type apiSnapShop struct {
	apiName string
	isPaused   bool
	errCount   int64
	totalCount int64

	accessLast int64 // last access timestamp of api
	roundLast  int64 // start timestamp of this round
}

type CircuitBreakerImp struct {
	lock            sync.RWMutex
	apiMap          map[string]*apiSnapShop // api mapping for your server
	minCheck        int64                   // lower limit of cbk
	cbkErrRate      float64
	recoverInterval time.Duration           // interval of cool down for api
	roundInterval   time.Duration           // interval for cbk to reset api
}

// accessed mark api status when invoked
func (c *CircuitBreakerImp) accessed(api *apiSnapShop) {
	/*
		to check round end?
		- yes: reset count
		- no: update count
	*/
	now := time.Now().UnixNano()
	if util.Abs64(now-api.roundLast) > int64(c.roundInterval) {
		if api.roundLast != 0 {
			log.Warnf("# Cbk round end, reset all metric for api:%s.", api.apiName)
		}
		api.errCount = 0
		api.totalCount = 0
		api.roundLast = now
	}
	api.totalCount++
	api.accessLast = now
}

// CanAccess check api whether can access,if cbk triggered,
// it should be cool down a period of time
func (c *CircuitBreakerImp) CanAccess(key string) bool {
	c.lock.RLock()
	defer c.lock.RUnlock()

	if api, ok := c.apiMap[key]; ok {
		log.Debugf("# Cbk detail for api id key, total: %v, "+
			"errCount: %v, paused: %v", api.totalCount,	api.errCount, api.isPaused)
		if api.isPaused {
			latency := util.Abs64(time.Now().UnixNano() - api.accessLast)
			if latency < int64(c.recoverInterval) {
				// 在恢复期之内, 保持熔断
				return false
			} else {
				log.Warnf("# Cool down enough time for %v, recover api access: %v.", c.recoverInterval, key)
			}
		}
	}
	return true
}


// Failed mark api access and init when first access in a round
func (c *CircuitBreakerImp) Failed(key string) {
	c.lock.Lock()
	defer c.lock.Unlock()

	if api, ok := c.apiMap[key]; ok {
		c.accessed(api)
		api.errCount++

		errRate := float64(api.errCount) / float64(api.totalCount)
		// both cover err rate and lower min of access count
		if api.totalCount > c.minCheck && errRate > c.cbkErrRate {
			log.Warnf("# Trigger API %s errRate comes to %.3f, totalCount: %v cbk triggered!", key, errRate, api.totalCount)
			api.isPaused = true
		}
	} else {
		api := &apiSnapShop{}
		c.accessed(api)
		api.errCount++
		c.apiMap[key] = api
	}
}

// Succeed mark api access and turn off cbk when is paused
func (c *CircuitBreakerImp) Succeed(key string) {
	c.lock.Lock()
	c.lock.Unlock()

	if api, ok := c.apiMap[key]; ok {
		c.accessed(api)
		if api.isPaused {
			log.Warnf("# Trigger API %v succeed, set as allowed.", key)
			api.isPaused = false
		}
	}
}
