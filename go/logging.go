package main

import (
	"io"
	"os"
	"strings"

	"github.com/sirupsen/logrus"
	client "github.com/zelenin/go-tdlib/client"
	"gopkg.in/ini.v1"
	"gopkg.in/natefinch/lumberjack.v2"
)

var (
	coreLog   *logrus.Entry
	pjsipLog  *logrus.Entry
	tgvoipLog *logrus.Entry
	logFile   *lumberjack.Logger
)

// sipMessages controls whether full SIP messages are logged.
var sipMessages bool

// initLogging configures loggers similar to the C++ version.
func initLogging(cfg *ini.File) error {
	sec := cfg.Section("logging")

	consoleMin := toLogrusLevel(sec.Key("console_min_level").MustInt(0))
	fileMin := toLogrusLevel(sec.Key("file_min_level").MustInt(0))

	logFile = &lumberjack.Logger{
		Filename:   "tg2sip.log",
		MaxSize:    100, // megabytes
		MaxBackups: 1,
	}

	coreLog = newLogger("core", toLogrusLevel(sec.Key("core").MustInt(2)), consoleMin, fileMin, logFile)
	pjsipLog = newLogger("pjsip", toLogrusLevel(sec.Key("pjsip").MustInt(2)), consoleMin, fileMin, logFile)
	tgvoipLog = newLogger("tgvoip", toLogrusLevel(sec.Key("tgvoip").MustInt(5)), consoleMin, fileMin, logFile)

	sipMessages = sec.Key("sip_messages").MustBool(true)
	if !sipMessages {
		// filter out verbose SIP message dumps
		pjsipLog.Logger.AddHook(&sipMessageFilterHook{})
	}

	// configure TDLib logging
	tdlibLevel := int32(sec.Key("tdlib").MustInt(3))
	if _, err := client.SetLogStream(&client.SetLogStreamRequest{LogStream: &client.LogStreamFile{Path: "tdlib.log", MaxFileSize: 100 * 1024 * 1024}}); err != nil {
		return err
	}
	_, err := client.SetLogVerbosityLevel(&client.SetLogVerbosityLevelRequest{NewVerbosityLevel: tdlibLevel})
	return err
}

// closeLogging flushes and closes log files.
func closeLogging() {
	if logFile != nil {
		_ = logFile.Close()
	}
	_, _ = client.SetLogStream(&client.SetLogStreamRequest{LogStream: &client.LogStreamEmpty{}})
}

// writerHook writes logs to the specified writer for provided levels.
type writerHook struct {
	Writer    io.Writer
	LogLevels []logrus.Level
}

func (h *writerHook) Fire(e *logrus.Entry) error {
	line, err := e.String()
	if err != nil {
		return err
	}
	_, err = h.Writer.Write([]byte(line))
	return err
}

func (h *writerHook) Levels() []logrus.Level {
	return h.LogLevels
}

func newLogger(name string, level, consoleMin, fileMin logrus.Level, file io.Writer) *logrus.Entry {
	logger := logrus.New()
	logger.SetLevel(level)
	logger.SetOutput(io.Discard)
	logger.SetFormatter(&logrus.TextFormatter{FullTimestamp: true, TimestampFormat: "15:04:05.000"})
	logger.AddHook(&writerHook{Writer: os.Stdout, LogLevels: availableLevels(consoleMin)})
	logger.AddHook(&writerHook{Writer: file, LogLevels: availableLevels(fileMin)})
	return logger.WithField("name", name)
}

func availableLevels(min logrus.Level) []logrus.Level {
	levels := []logrus.Level{}
	for _, l := range logrus.AllLevels {
		if l <= min {
			levels = append(levels, l)
		}
	}
	return levels
}

func toLogrusLevel(v int) logrus.Level {
	switch {
	case v <= 0:
		return logrus.TraceLevel
	case v == 1:
		return logrus.DebugLevel
	case v == 2:
		return logrus.InfoLevel
	case v == 3:
		return logrus.WarnLevel
	case v == 4:
		return logrus.ErrorLevel
	case v == 5:
		return logrus.FatalLevel
	default:
		return logrus.PanicLevel // off
	}
}

// sipMessageFilterHook suppresses logging of full SIP messages when disabled via configuration.
type sipMessageFilterHook struct{}

func (h *sipMessageFilterHook) Levels() []logrus.Level { return logrus.AllLevels }

func (h *sipMessageFilterHook) Fire(e *logrus.Entry) error {
	if strings.HasPrefix(e.Message, "received SIP message:") {
		// elevate level so writer hooks ignore the entry
		e.Level = logrus.PanicLevel + 1
	}
	return nil
}
