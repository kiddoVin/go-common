# go-common
some common modules for golang

## Logger

一个简单的Logger模块，除了普通的日志打印功能，它有几个小的特点：

  * 支持日志切分，提供了切分控制函数，可以自行比如挂上信号量/time.Ticker雷来控制切分
  * 日志格式开放，可以按照自己需要完全自定义日志格式
  * 支持业务上下文，能够注入类似LogId，moduleName等信息到日志中去

得益于golang的特性，使用channel作为日志管道，免去了很多并发问题，代码量很少。请直接阅读代码吧。
