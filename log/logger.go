package log

import (
	"errors"
	"fmt"
	"math"
	"os"
	"path"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

const MAX_LOG_BUFFER_SIZE int = 4096

const (
	LOG_LEVEL_DEBUG uint64 = 1 << iota
	LOG_LEVEL_INFO
	LOG_LEVEL_TRACE
	LOG_LEVEL_NOTICE
	LOG_LEVEL_WARNING
	LOG_LEVEL_FATAL
)

const (
	LOG_CONTROL_ROTATE int = 1 << iota
	LOG_CONTROL_STOP
)

var LOG_LEVEL_STRING = map[uint64]string{
	LOG_LEVEL_DEBUG:   "[DEBUG]",
	LOG_LEVEL_INFO:    "[INFO]",
	LOG_LEVEL_TRACE:   "[TRACE]",
	LOG_LEVEL_NOTICE:  "\033[34m[NOTICE]\033[0m",
	LOG_LEVEL_WARNING: "\033[33m[WARNING]\033[0m",
	LOG_LEVEL_FATAL:   "\033[31m[FATAL]\033[0m",
}

const (
	NORMAL_LOG_FLAG = LOG_LEVEL_DEBUG | LOG_LEVEL_INFO | LOG_LEVEL_TRACE | LOG_LEVEL_NOTICE
	WF_LOG_FLAG     = LOG_LEVEL_WARNING | LOG_LEVEL_FATAL
)

type LogHandle struct {
	logBuffer chan string

	logControl chan int

	file *os.File

	logFile logFileInfo

	originFilePath string

	flag uint64

	isRotate bool

	isRunning bool

	waitGroup sync.WaitGroup
}

type logFileInfo struct {
	logPath string

	logFile string
}

type Logger struct {
	logHandleList []*LogHandle
}

func CreateFileLogHandle(logPath string, logFile string, flag uint64, isRotate bool) *LogHandle {

	file, err := os.OpenFile(logPath+logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		panic(err)
	}

	logHandle := &LogHandle{
		logBuffer:  make(chan string, MAX_LOG_BUFFER_SIZE),
		logControl: make(chan int, 1),
		file:       file,
		flag:       flag,
		isRotate:   isRotate,
		isRunning:  false,
		logFile: logFileInfo{
			logPath: logPath,
			logFile: logFile,
		},
	}

	return logHandle
}

func (l *Logger) AddLogHandle(logHandle *LogHandle) {
	l.logHandleList = append(l.logHandleList, logHandle)
}

func (l Logger) StartLogger() {
	for _, logHandle := range l.logHandleList {
		logHandle.startOutPut()
	}
}

func (l Logger) StopLogger() {
	for _, logHandle := range l.logHandleList {
		logHandle.stopOutPut()
	}
}

func (l Logger) Rotate() {
	for _, logHandle := range l.logHandleList {
		logHandle.logControl <- LOG_CONTROL_ROTATE
	}
}

func (l Logger) writeLog(logLevel uint64, logString string) {
	for _, logHandle := range l.logHandleList {
		if (logHandle.flag & logLevel) != 0 {
			logHandle.logBuffer <- logString
		}
	}
}

func buildLog(stackDeep int, prefixString string, format string, v ...interface{}) string {

	// 原始日志
	msg := fmt.Sprintf(format, v...)

	// 时间相关
	nowTime := time.Now().Format("2006-01-02 15:04:05")

	// 文件行数
	_, file, line, ok := runtime.Caller(stackDeep)
	if !ok {
		file = "???"
		line = 0
	}

	// 去除原始日志里面的回车
	msg = strings.Replace(msg, "\n", " ", -1)

	_, filename := path.Split(file)
	fileLine := "[" + filename + ":" + strconv.FormatInt(int64(line), 10) + "]"

	return fmt.Sprintf("%s %s %s %s\n", prefixString, nowTime, fileLine, msg)
}

func (l Logger) Write(p []byte) (n int, err error) {
	l.writeLog(NORMAL_LOG_FLAG, buildLog(2, "\033[36m[XORM]\033[0m", "%s", string(p)))
	return len(p), nil
}

func (l Logger) Print(v ...interface{}) {
	l.writeLog(NORMAL_LOG_FLAG, buildLog(2, "\033[34m[KAFKA]\033[0m", "%s", fmt.Sprint(v)))
}

func (l Logger) Printf(format string, v ...interface{}) {
	l.writeLog(NORMAL_LOG_FLAG, buildLog(2, "\033[34m[KAFKA]\033[0m", "%s", fmt.Sprintf(format, v...)))
}

func (l Logger) Println(v ...interface{}) {
	l.writeLog(NORMAL_LOG_FLAG, buildLog(2, "\033[34m[KAFKA]\033[0m", "%s", fmt.Sprintln(v...)))
}

func (l Logger) Fatal(format string, v ...interface{}) {
	l.writeLog(LOG_LEVEL_FATAL, buildLog(2, LOG_LEVEL_STRING[LOG_LEVEL_FATAL], format, v...))
}

func (l Logger) Warning(format string, v ...interface{}) {
	l.writeLog(LOG_LEVEL_WARNING, buildLog(2, LOG_LEVEL_STRING[LOG_LEVEL_WARNING], format, v...))
}

func (l Logger) Notice(format string, v ...interface{}) {
	l.writeLog(LOG_LEVEL_NOTICE, buildLog(2, LOG_LEVEL_STRING[LOG_LEVEL_NOTICE], format, v...))
}

func (l Logger) Info(format string, v ...interface{}) {
	l.writeLog(LOG_LEVEL_INFO, buildLog(2, LOG_LEVEL_STRING[LOG_LEVEL_INFO], format, v...))
}

func (l Logger) Trace(format string, v ...interface{}) {
	l.writeLog(LOG_LEVEL_TRACE, buildLog(2, LOG_LEVEL_STRING[LOG_LEVEL_TRACE], format, v...))
}

func (l Logger) Debug(format string, v ...interface{}) {
	l.writeLog(LOG_LEVEL_DEBUG, buildLog(2, LOG_LEVEL_STRING[LOG_LEVEL_DEBUG], format, v...))
}

func (logHandle *LogHandle) stopOutPut() {

	// 发送停止信号
	logHandle.logControl <- LOG_CONTROL_STOP

	// 等待其他线程结束
	logHandle.waitGroup.Wait()

	// 刷日志
LOG_LOOP:
	for {
		select {
		case logString := <-logHandle.logBuffer:
			logHandle.file.Write([]byte(logString))
		default:
			break LOG_LOOP
		}
	}

	// 关闭文件
	logHandle.file.Close()
}

func (logHandle *LogHandle) startOutPut() error {
	if true == logHandle.isRunning {
		return errors.New("logHandle is alreay running")
	}

	logHandle.isRunning = true

	logHandle.waitGroup.Add(1)

	go logHandle.run()

	return nil
}

func (logHandle *LogHandle) run() {
LOG_LOOP:
	for {
		select {
		case logString := <-logHandle.logBuffer:

			logHandle.file.Write([]byte(logString))

		case controlFlag := <-logHandle.logControl:

			switch controlFlag {

			case LOG_CONTROL_STOP:
				logHandle.isRunning = false
				break LOG_LOOP

			case LOG_CONTROL_ROTATE:
				logHandle.doRotate()
			}
		}
	}

	logHandle.waitGroup.Done()

}

func (logHandle *LogHandle) setRotate(allowedRotated bool) {
	logHandle.isRotate = allowedRotated
}

func (logHandle *LogHandle) doRotate() {

	if !logHandle.isRotate {
		return
	}

	nowTime := time.Now()

	lastHour := nowTime.Add(-1 * time.Hour).Hour()

	logSuffix := time.Date(nowTime.Year(), nowTime.Month(), nowTime.Day(),
		lastHour, 0, 0, 0, nowTime.Location()).Format("2006010215")

	logFileName := findFile(logHandle.logFile.logPath, logHandle.logFile.logFile, logSuffix)

	logFile := fmt.Sprintf("%s/%s", logHandle.logFile.logPath, logHandle.logFile.logFile)

	err := os.Rename(logFile, logFileName)

	if err != nil {
		fmt.Fprintf(os.Stderr, "[LOG_ROTATE] rename log file from %s to %s failed with error: %s\n",
			logFile, logFileName, err.Error())

	}

	newFile, err := os.OpenFile(logFile, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[LOG_ROTATE] open log file %s failed with error: %s\n",
			logFile, err.Error())

		return
	}

	logHandle.file.Close()

	logHandle.file = newFile
}

func findFile(filePath string, fileName string, fileSuffix string) string {

	filePathName := fmt.Sprintf("%s/%s", filePath, fileName)

	logFile := fmt.Sprintf("%s.%s", filePathName, fileSuffix)

	_, err := os.Lstat(logFile)

	if os.IsNotExist(err) {
		return logFile
	}

	for index := 0; index < math.MinInt64; index++ {
		logFileWithNumber := fmt.Sprintf("%s.%s.%d", filePathName, fileSuffix, index)

		_, err := os.Lstat(logFileWithNumber)

		if os.IsNotExist(err) {
			return logFileWithNumber
		}
	}

	return logFile
}

type LogBuilder interface {
	BuildLog(stackDeep int, logLevel uint64, prefixKey map[interface{}]interface{},
		format string, v ...interface{}) string
}

func CreateContextLogger(logDeep int, model interface{}, logId interface{}) *ContextLogger {
	return &ContextLogger{
		RealLogger: &Log,
		PrefixKey: map[interface{}]interface{}{
			"LogId": logId,
			"model": model,
		},
		Deep:               logDeep + 3,
		LogBuilderInstance: DefaultLogBuilder{},
	}
}

// 上下文相关的Logger
type ContextLogger struct {

	// 真实日志
	RealLogger *Logger

	// 日志上线文
	PrefixKey map[interface{}]interface{}

	// 日志深度
	Deep int

	// 日志构建
	LogBuilderInstance LogBuilder
}

func (l ContextLogger) Fatal(format string, v ...interface{}) {
	l.RealLogger.writeLog(LOG_LEVEL_FATAL,
		l.LogBuilderInstance.BuildLog(l.Deep, LOG_LEVEL_FATAL, l.PrefixKey, format, v...))
}

func (l ContextLogger) Warning(format string, v ...interface{}) {
	l.RealLogger.writeLog(LOG_LEVEL_WARNING,
		l.LogBuilderInstance.BuildLog(l.Deep, LOG_LEVEL_WARNING, l.PrefixKey, format, v...))
}

func (l ContextLogger) Notice(format string, v ...interface{}) {
	l.RealLogger.writeLog(LOG_LEVEL_NOTICE,
		l.LogBuilderInstance.BuildLog(l.Deep, LOG_LEVEL_NOTICE, l.PrefixKey, format, v...))
}

func (l ContextLogger) Info(format string, v ...interface{}) {
	l.RealLogger.writeLog(LOG_LEVEL_INFO,
		l.LogBuilderInstance.BuildLog(l.Deep, LOG_LEVEL_INFO, l.PrefixKey, format, v...))
}

func (l ContextLogger) Trace(format string, v ...interface{}) {
	l.RealLogger.writeLog(LOG_LEVEL_TRACE,
		l.LogBuilderInstance.BuildLog(l.Deep, LOG_LEVEL_TRACE, l.PrefixKey, format, v...))
}

func (l ContextLogger) Debug(format string, v ...interface{}) {
	l.RealLogger.writeLog(LOG_LEVEL_DEBUG,
		l.LogBuilderInstance.BuildLog(l.Deep, LOG_LEVEL_DEBUG, l.PrefixKey, format, v...))
}

type DefaultLogBuilder struct {
}

func (d DefaultLogBuilder) BuildLog(stackDeep int, logLevel uint64, prefixKey map[interface{}]interface{}, format string, v ...interface{}) string {

	// 原始日志
	msg := fmt.Sprintf(format, v...)

	// 时间相关
	nowTime := time.Now().Format("2006-01-02 15:04:05")

	// 文件行数
	_, file, line, ok := runtime.Caller(stackDeep)
	if !ok {
		file = "???"
		line = 0
	}

	// 去除原始日志里面的回车
	msg = strings.Replace(msg, "\n", " ", -1)

	// 当前文件
	_, filename := path.Split(file)
	fileLine := "[" + filename + ":" + strconv.FormatInt(int64(line), 10) + "]"

	// 日志上下文
	prefixStringArray := []string{}
	for key, value := range prefixKey {
		prefixStringArray = append(prefixStringArray, "["+fmt.Sprint(key)+":"+fmt.Sprint(value)+"]")
	}

	prefixString := strings.Join(prefixStringArray, " ")

	return fmt.Sprintf("%s %s %s %s %s\n", LOG_LEVEL_STRING[logLevel], nowTime, fileLine, prefixString, msg)
}

// how to use
var Log Logger

func init() {
	// 创建标准输出日志
	stdOutHandle := &LogHandle{
		logBuffer:  make(chan string, MAX_LOG_BUFFER_SIZE),
		logControl: make(chan int, 1),
		file:       os.Stdout,
		flag:       NORMAL_LOG_FLAG | WF_LOG_FLAG,
		isRotate:   false,
		isRunning:  false,
		logFile: logFileInfo{
			logPath: "",
			logFile: "",
		},
	}

	// 所有的日志，给一份给标准输出
	Log.AddLogHandle(stdOutHandle)

	// 一份正常文件
	Log.AddLogHandle(CreateFileLogHandle("./logs",
		"test.log", NORMAL_LOG_FLAG, true))

	// 一份异常日志
	Log.AddLogHandle(CreateFileLogHandle("./logs",
		"test.log.wf", WF_LOG_FLAG, true))

	// 启动日志输出
	Log.StartLogger()

	// 打印日志
	Log.Info("log init finished")
}
