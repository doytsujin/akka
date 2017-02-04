package event

import (
	"fmt"
	"github.com/go-akka/akka"
	"github.com/go-akka/akka/actor/props"
	"github.com/go-akka/akka/pkg/class_loader"
	"reflect"
	"sync/atomic"
	"time"
)

var (
	_loggerId int64 = 0
)

type LoggingBus struct {
	akka.EventBus

	loggers  []akka.ActorRef
	logLevel akka.LogLevel
}

func NewLoggingBus(classification akka.EventBus) *LoggingBus {
	return &LoggingBus{
		EventBus: classification,
	}
}

func (p *LoggingBus) SetLogLevel(logLevel akka.LogLevel) {
	p.logLevel = logLevel

	for _, logger := range p.loggers {
		p.subscribeLogLevelAndAbove(logLevel, logger)

		for _, level := range akka.AllLogLevels() {
			if level < logLevel {
				p.TUnsubscribe(logger, (*akka.LogLevel)(nil))
			}
		}
	}
}

func (p *LoggingBus) LogLevel() akka.LogLevel {
	return p.logLevel
}

func (p *LoggingBus) StartStdoutLogger(config *akka.Settings) {
	p.setUpStdoutLogger(config)
	p.Publish(NewDebugEvent(simpleName(p), p, "StandardOutLogger started"))

}

func (p *LoggingBus) StartDefaultLoggers(system akka.ActorSystemImpl) (err error) {
	logName := simpleName(p) + "(" + system.Name() + ")"
	logLevel := akka.LogLevelFor(system.Settings().LogLevel)
	loggerTypes := system.Settings().Loggers
	timeout := system.Settings().LoggerStartTimeout
	shouldRemoveStandardOutLogger := true

	for _, strLoggerType := range loggerTypes {
		loggerType, exist := class_loader.Default.ClassNameOf(strLoggerType)
		if !exist {
			panic("Logger specified in config cannot be found: " + strLoggerType)
		}

		if loggerType == StandardOutLoggerType {
			shouldRemoveStandardOutLogger = false
			continue
		}

		err = p.addLogger(system, loggerType, logLevel, logName, timeout)
		if err != nil {
			return
		}
	}

	// if system.Settings().DebugUnhandledMessage {
	// 	forwarder:=system.SystemActorOf(props.Create(v, ...), name)
	// }

	if shouldRemoveStandardOutLogger {
		p.Publish(NewDebugEvent(logName, p, "StandardOutLogger being removed"))
		p.TUnsubscribe(StandardOutLoggerInstance)
	}

	p.Publish(NewDebugEvent(logName, p, "Default Loggers started"))

	return
}

func (p *LoggingBus) addLogger(system akka.ActorSystemImpl, loggerType reflect.Type, logLevel akka.LogLevel, loggingBusName string, timeout time.Duration) error {
	loggerName := p.createLoggerName(loggerType)
	props, err := props.Create(loggerType)
	loggerProps := props.WithDispatcher(system.Settings().LoggersDispatcher)
	if err != nil {
		return err
	}

	var loggerActorRef akka.ActorRef
	loggerActorRef, err = system.SystemActorOf(loggerProps, loggerName)
	if err != nil {
		return err
	}

	// TODO: inital timeout

	p.loggers = append(p.loggers, loggerActorRef)
	p.subscribeLogLevelAndAbove(logLevel, loggerActorRef)
	p.Publish(NewDebugEvent(loggingBusName, p, fmt.Sprintf("Logger %s [%s] started", loggerName, simpleName(loggerType))))

	return nil

}

func (p *LoggingBus) setUpStdoutLogger(config *akka.Settings) {
	logLevel := akka.LogLevelFor(config.StdoutLogLevel)
	p.subscribeLogLevelAndAbove(logLevel, StandardOutLoggerInstance)

}

func (p *LoggingBus) subscribeLogLevelAndAbove(logLevel akka.LogLevel, logger akka.ActorRef) {
	for _, level := range akka.AllLogLevels() {
		if level >= logLevel {
			p.TSubscribe(logger, LogClassFor(logLevel))
		}
	}
}

func (p *LoggingBus) createLoggerName(actor interface{}) string {
	id := atomic.AddInt64(&_loggerId, 1)
	name := fmt.Sprintf("log%d-%s", id, simpleName(actor))
	return name
}
