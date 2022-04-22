package http_default

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	. "github.com/chefsgo/base"
	"github.com/chefsgo/chef"
	"github.com/gorilla/mux"
)

//------------------------- 默认事件驱动 begin --------------------------

const (
	defaultHttpSeparator = "|||"
)

type (
	defaultHttpDriver  struct{}
	defaultHttpConnect struct {
		mutex   sync.RWMutex
		actives int64

		config chef.HttpConfig

		server  *http.Server
		router  *mux.Router
		handler chef.HttpHandler

		routes    map[string]*mux.Route
		registers map[string]chef.HttpRegister
	}

	//响应对象
	defaultHttpThread struct {
		connect  *defaultHttpConnect
		name     string
		site     string
		params   Map
		request  *http.Request
		response http.ResponseWriter
	}
)

//连接
func (driver *defaultHttpDriver) Connect(config chef.HttpConfig) (chef.HttpConnect, error) {
	return &defaultHttpConnect{
		config:    config,
		routes:    map[string]*mux.Route{},
		registers: map[string]chef.HttpRegister{},
	}, nil
}

//打开连接
func (connect *defaultHttpConnect) Open() error {
	connect.router = mux.NewRouter()
	connect.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", connect.config.Port),
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      connect.router,
	}

	return nil
}
func (connect *defaultHttpConnect) Health() (chef.HttpHealth, error) {
	//connect.mutex.RLock()
	//defer connect.mutex.RUnlock()
	return chef.HttpHealth{Workload: connect.actives}, nil
}

//关闭连接
func (connect *defaultHttpConnect) Close() error {

	//安全关闭连接来
	// c := make(chan os.Signal, 1)
	// // We'll accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// // SIGKILL, SIGQUIT or SIGTERM (Ctrl+/) will not be caught.
	// signal.Notify(c, os.Interrupt)

	// // Block until we receive our signal.
	// <-c

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	// Doesn't block if no connections, but will otherwise wait
	// until the timeout deadline.
	connect.server.Shutdown(ctx)
	// // Optionally, you could run srv.Shutdown in a goroutine and block on
	// // <-ctx.Done() if your application should wait for other services
	// // to finalize based on context cancellation.
	// log.Println("shutting down")
	// os.Exit(0)

	return nil
}

//注册回调
func (connect *defaultHttpConnect) Accept(handler chef.HttpHandler) error {
	connect.mutex.Lock()
	defer connect.mutex.Unlock()

	//保存回调
	connect.handler = handler

	//先注册一个接入全部请求的
	connect.router.NotFoundHandler = connect
	connect.router.MethodNotAllowedHandler = connect

	return nil
}

//订阅者，注册事件
func (connect *defaultHttpConnect) Register(name string, config chef.HttpRegister) error {
	connect.mutex.Lock()
	defer connect.mutex.Unlock()

	//如果hosts为空，加一个空字串，用于循环，就是一定会进入循环
	// if len(config.Hosts) == 0 {
	// 	config.Hosts = append(config.Hosts, "")
	// }

	for i, uri := range config.Uris {
		routeName := fmt.Sprintf("%s%s%v", name, defaultHttpSeparator, i)

		route := connect.router.HandleFunc(uri, connect.ServeHTTP).Name(routeName)
		route.HandlerFunc(connect.ServeHTTP)
		for _, host := range config.Hosts {
			route.Host(host)
		}

		if len(config.Methods) > 0 {
			route.Methods(config.Methods...)
		}

		connect.routes[name] = route
	}

	//开工支持多个uri和多域名
	// for i, uri := range config.Uris {
	// 	for j, host := range config.Hosts {

	// 		//自定义路由名称
	// 		//Serve的时候，去掉/后面的内容，来匹配
	// 		routeName := fmt.Sprintf("%s%s%v.%v", name, defaultHttpSeparator, i, j)
	// 		route := connect.router.HandleFunc(uri, connect.ServeHTTP).Name(routeName)
	// 		if len(config.Methods) > 0 {
	// 			route.Methods(config.Methods...)
	// 		}
	// 		if host != "" {
	// 			//这里有个问题，当部署的时候非80，而前端走nginx反向的时候是80的时候，就匹配不上
	// 			//因为nginx是绑的80，而实际的节点，不是跑在80端口，这里就有问题了
	// 			//所以设置只在开放模式下有效， 测试和生产，都直接使用80端口
	// 			if chef.Mode == Developing && connect.config.Port != 80 {
	// 				host = fmt.Sprintf("%v:%v", host, connect.config.Port)
	// 			}

	// 			//chef.Debug("http.reg", routeName, host)

	// 			route.Host(host)
	// 		}

	// 		connect.routes[routeName] = route
	// 	}
	// }

	connect.registers[name] = config

	return nil
}

func (connect *defaultHttpConnect) Start() error {
	if connect.server == nil {
		panic("[HTTP]请先打开连接")
	}

	go func() {
		err := connect.server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			panic(err.Error())
		}
	}()

	return nil
}
func (connect *defaultHttpConnect) StartTLS(certFile, keyFile string) error {
	if connect.server == nil {
		panic("[HTTP]请先打开连接")
	}

	go func() {
		err := connect.server.ListenAndServeTLS(certFile, keyFile)
		if err != nil && err != http.ErrServerClosed {
			panic(err.Error())
		}
	}()

	return nil
}

func (connect *defaultHttpConnect) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	name := ""
	site := ""
	params := Map{}

	//有一个特别诡异的问题
	//直接从包加载的，没有问题
	//如果是从so文件加载的，这里会获取不到route
	//而先从包加载一次，再从so加载， 就可以获取到route了
	route := mux.CurrentRoute(req)

	if route != nil {
		name = route.GetName()

		names := strings.Split(name, defaultHttpSeparator)
		if len(names) >= 2 {
			name = names[0]
		}

		if regis, ok := connect.registers[name]; ok {
			site = regis.Site
		}

		vars := mux.Vars(req)
		for k, v := range vars {
			params[k] = v
		}
	}

	//chef.Debug("serve", name, site, params)

	connect.request(name, site, params, res, req)
}

//servehttp
func (connect *defaultHttpConnect) request(name, site string, params Map, res http.ResponseWriter, req *http.Request) {
	if connect.handler != nil {
		connect.handler(&defaultHttpThread{
			connect, name, site, params, req, res,
		})
	}
}

func (thread *defaultHttpThread) Site() string {
	return thread.site
}
func (thread *defaultHttpThread) Name() string {
	return thread.name
}
func (thread *defaultHttpThread) Params() Map {
	return thread.params
}
func (thread *defaultHttpThread) Request() *http.Request {
	return thread.request
}
func (thread *defaultHttpThread) Response() http.ResponseWriter {
	return thread.response
}

//完成
func (res *defaultHttpThread) Finish() error {
	return nil
}

//------------------------- 默认HTTP驱动 end --------------------------
