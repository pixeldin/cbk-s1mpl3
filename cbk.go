package cbk_s1mpl3

// Cbk接口
type CircuitBreaker interface {
	CanAccess(key string) bool
	// mark api failed, and cool down when with high err rate
	Failed(key string)
	// mark api succeed
	Succeed(key string)
	//Status() interface{}
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
