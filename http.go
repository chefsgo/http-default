package http_default

import (
	"context"
	"fmt"
	nethttp "net/http"
	"sync"
	"time"

	. "github.com/chefsgo/base"
	"github.com/chefsgo/http"
	"github.com/gorilla/mux"
)

//------------------------- 默认事件驱动 begin --------------------------

const (
	defaultSeparator = "|||"
)

type (
	defaultDriver  struct{}
	defaultConnect struct {
		mutex   sync.RWMutex
		actives int64

		config http.Config

		server *nethttp.Server
		router *mux.Router

		delegate http.Delegate

		routes map[string]*mux.Route
	}

	//响应对象
	defaultThread struct {
		connect  *defaultConnect
		name     string
		site     string
		params   Map
		request  *nethttp.Request
		response nethttp.ResponseWriter
	}
)

//连接
func (driver *defaultDriver) Connect(config http.Config) (http.Connect, error) {
	return &defaultConnect{
		config: config, routes: map[string]*mux.Route{},
	}, nil
}

//打开连接
func (connect *defaultConnect) Open() error {
	connect.router = mux.NewRouter()
	connect.server = &nethttp.Server{
		Addr:         fmt.Sprintf(":%d", connect.config.Port),
		WriteTimeout: time.Second * 15,
		ReadTimeout:  time.Second * 15,
		IdleTimeout:  time.Second * 60,
		Handler:      connect.router,
	}

	return nil
}
func (connect *defaultConnect) Health() (http.Health, error) {
	//connect.mutex.RLock()
	//defer connect.mutex.RUnlock()
	return http.Health{Workload: connect.actives}, nil
}

//关闭连接
func (connect *defaultConnect) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*5)
	defer cancel()
	return connect.server.Shutdown(ctx)
}

//注册回调
func (connect *defaultConnect) Accept(delegate http.Delegate) error {
	connect.mutex.Lock()
	defer connect.mutex.Unlock()

	//保存回调
	connect.delegate = delegate

	//先注册一个接入全部请求的
	connect.router.NotFoundHandler = connect
	connect.router.MethodNotAllowedHandler = connect

	return nil
}

//订阅者，注册事件
func (connect *defaultConnect) Register(name string, info http.Info) error {
	connect.mutex.Lock()
	defer connect.mutex.Unlock()

	route := connect.router.HandleFunc(info.Uri, connect.ServeHTTP).Name(name)
	for _, host := range info.Hosts {
		route.Host(host)
	}
	if info.Method != "" {
		route.Methods(info.Method)
	}

	connect.routes[name] = route

	return nil
}

func (connect *defaultConnect) Start() error {
	if connect.server == nil {
		panic("Invalid http connect.")
	}

	go func() {
		err := connect.server.ListenAndServe()
		if err != nil && err != nethttp.ErrServerClosed {
			panic(err.Error())
		}
	}()

	return nil
}
func (connect *defaultConnect) StartTLS(certFile, keyFile string) error {
	if connect.server == nil {
		panic("Invalid http connect.")
	}

	go func() {
		err := connect.server.ListenAndServeTLS(certFile, keyFile)
		if err != nil && err != nethttp.ErrServerClosed {
			panic(err.Error())
		}
	}()

	return nil
}

func (connect *defaultConnect) ServeHTTP(res nethttp.ResponseWriter, req *nethttp.Request) {
	name := ""
	params := Map{}

	route := mux.CurrentRoute(req)

	if route != nil {
		name = route.GetName()
		vars := mux.Vars(req)
		for k, v := range vars {
			params[k] = v
		}
	}

	// 有请求都发，404也转过去
	connect.delegate.Serve(name, params, res, req)
}

//------------------------- 默认HTTP驱动 end --------------------------
