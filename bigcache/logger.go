package bigcache

import (
	"log"
	"os"
)

// Logger is invoked when `Config.Verbose=true`
// 当config.Verbose==true 的时候 Logger 被用到
type Logger interface {
	Printf(format string, v ...interface{})
}

// this is a safeguard, breaking on compile time in case
// `log.Logger` does not adhere to our `Logger` interface.
// see https://golang.org/doc/faq#guarantee_satisfies_interface
// 就是约定log.Logger（struct）必须实现 Logger 接口。如果没实现，会在编译期间报错
var _ Logger = &log.Logger{}

// DefaultLogger returns a `Logger` implementation
// backed by stdlib's log
// DefaultLogger返回由stdlib的日志支持的`Logger`实现
func DefaultLogger() *log.Logger {
	return log.New(os.Stdout, "", log.LstdFlags)
}

func newLogger(custom Logger) Logger {
	if custom != nil {
		return custom
	}

	return DefaultLogger()
}
