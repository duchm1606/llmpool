package handler

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const callbackForwarderShutdownTimeout = 2 * time.Second

type oauthCallbackForwarder interface {
	Start(state string, listenPort int, listenPath string, targetBase string) error
	StopByState(state string)
}

type localCallbackForwarder struct {
	mu      sync.Mutex
	ttl     time.Duration
	byState map[string]*callbackForwarderInstance
	byPort  map[int]*callbackForwarderInstance
}

type callbackForwarderInstance struct {
	port   int
	state  string
	server *http.Server
	done   chan struct{}
}

var (
	callbackForwarderRegistryMu sync.Mutex
	callbackForwarderRegistry   = make(map[int]*callbackForwarderInstance)
)

func newLocalCallbackForwarder(ttl time.Duration) *localCallbackForwarder {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}

	return &localCallbackForwarder{
		ttl:     ttl,
		byState: make(map[string]*callbackForwarderInstance),
		byPort:  make(map[int]*callbackForwarderInstance),
	}
}

func (f *localCallbackForwarder) Start(state string, listenPort int, listenPath string, targetBase string) error {
	state = strings.TrimSpace(state)
	if state == "" {
		return fmt.Errorf("state is required")
	}

	if listenPort <= 0 || listenPort > 65535 {
		return fmt.Errorf("invalid listen port: %d", listenPort)
	}

	path := strings.TrimSpace(listenPath)
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	target := strings.TrimSpace(targetBase)
	if target == "" {
		return fmt.Errorf("target base URL is required")
	}

	previous := f.popByPort(listenPort)
	registryPrevious := detachCallbackForwarderFromRegistry(listenPort)
	f.stopInstance(previous)
	if registryPrevious != previous {
		f.stopInstance(registryPrevious)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", listenPort)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}

	mux := http.NewServeMux()
	mux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
		redirectTo := appendRawQuery(target, r.URL.RawQuery)
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, redirectTo, http.StatusFound)
	})

	server := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		WriteTimeout:      5 * time.Second,
	}

	instance := &callbackForwarderInstance{
		port:   listenPort,
		state:  state,
		server: server,
		done:   make(chan struct{}),
	}

	go func() {
		if errServe := server.Serve(listener); errServe != nil && !errors.Is(errServe, http.ErrServerClosed) {
			_ = errServe
		}
		close(instance.done)
	}()

	f.mu.Lock()
	f.byState[state] = instance
	f.byPort[listenPort] = instance
	f.mu.Unlock()

	setCallbackForwarderInRegistry(listenPort, instance)

	go f.stopAfterTTL(state, instance)

	return nil
}

func (f *localCallbackForwarder) StopByState(state string) {
	state = strings.TrimSpace(state)
	if state == "" {
		return
	}

	f.mu.Lock()
	instance := f.byState[state]
	if instance != nil {
		delete(f.byState, state)
		for port, current := range f.byPort {
			if current == instance {
				delete(f.byPort, port)
			}
		}
	}
	f.mu.Unlock()

	removeCallbackForwarderFromRegistry(instance)

	f.stopInstance(instance)
}

func (f *localCallbackForwarder) stopAfterTTL(state string, expected *callbackForwarderInstance) {
	timer := time.NewTimer(f.ttl)
	defer timer.Stop()

	<-timer.C

	f.mu.Lock()
	current := f.byState[state]
	if current != expected {
		f.mu.Unlock()
		return
	}

	delete(f.byState, state)
	for port, instance := range f.byPort {
		if instance == expected {
			delete(f.byPort, port)
		}
	}
	f.mu.Unlock()

	removeCallbackForwarderFromRegistry(expected)

	f.stopInstance(expected)
}

func (f *localCallbackForwarder) popByPort(port int) *callbackForwarderInstance {
	f.mu.Lock()
	defer f.mu.Unlock()

	instance := f.byPort[port]
	if instance == nil {
		return nil
	}

	delete(f.byPort, port)
	for state, current := range f.byState {
		if current == instance {
			delete(f.byState, state)
		}
	}

	return instance
}

func (f *localCallbackForwarder) stopInstance(instance *callbackForwarderInstance) {
	if instance == nil || instance.server == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), callbackForwarderShutdownTimeout)
	defer cancel()

	_ = instance.server.Shutdown(ctx)

	select {
	case <-instance.done:
	case <-time.After(callbackForwarderShutdownTimeout):
	}
}

func setCallbackForwarderInRegistry(port int, instance *callbackForwarderInstance) {
	if instance == nil {
		return
	}

	callbackForwarderRegistryMu.Lock()
	callbackForwarderRegistry[port] = instance
	callbackForwarderRegistryMu.Unlock()
}

func detachCallbackForwarderFromRegistry(port int) *callbackForwarderInstance {
	callbackForwarderRegistryMu.Lock()
	defer callbackForwarderRegistryMu.Unlock()

	instance := callbackForwarderRegistry[port]
	if instance != nil {
		delete(callbackForwarderRegistry, port)
	}

	return instance
}

func removeCallbackForwarderFromRegistry(instance *callbackForwarderInstance) {
	if instance == nil {
		return
	}

	callbackForwarderRegistryMu.Lock()
	defer callbackForwarderRegistryMu.Unlock()

	current := callbackForwarderRegistry[instance.port]
	if current == instance {
		delete(callbackForwarderRegistry, instance.port)
	}
}

func appendRawQuery(targetBase string, rawQuery string) string {
	if strings.TrimSpace(rawQuery) == "" {
		return targetBase
	}

	if strings.Contains(targetBase, "?") {
		return targetBase + "&" + rawQuery
	}

	return targetBase + "?" + rawQuery
}
