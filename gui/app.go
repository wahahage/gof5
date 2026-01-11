package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/kayrus/gof5/pkg/client"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx        context.Context
	cancelFunc context.CancelFunc
	wg         sync.WaitGroup
}

// NewApp creates a new App application struct
func NewApp() *App {
	return &App{}
}

// LogEmitter captures logs and emits them to frontend
type LogEmitter struct {
	ctx context.Context
}

func (l *LogEmitter) Write(p []byte) (n int, err error) {
	if l.ctx != nil {
		runtime.EventsEmit(l.ctx, "log", string(p))
	} else {
		fmt.Print(string(p))
	}
	return len(p), nil
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	log.SetOutput(&LogEmitter{ctx: ctx})
}

// Connect starts the VPN connection
func (a *App) Connect(server, username, password string) error {
	if a.cancelFunc != nil {
		return fmt.Errorf("already connected")
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.cancelFunc = cancel

	opts := client.Options{
		Server:   server,
		Username: username,
		Password: password,
		Debug:    true,
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()

		runtime.EventsEmit(a.ctx, "status", "Connecting...")
		err := client.Connect(ctx, &opts)
		if err != nil {
			if ctx.Err() == context.Canceled {
				runtime.EventsEmit(a.ctx, "status", "Disconnected")
				runtime.EventsEmit(a.ctx, "log", "VPN session ended.\n")
			} else {
				runtime.EventsEmit(a.ctx, "error", err.Error())
				runtime.EventsEmit(a.ctx, "status", "Error")
			}
		} else {
			runtime.EventsEmit(a.ctx, "status", "Disconnected")
			runtime.EventsEmit(a.ctx, "log", "VPN session ended.\n")
		}

		// Reset cancelFunc if it matches our context, but it's tricky without lock.
		// For simplicity, we assume one connection at a time.
		// If implementation of Connect blocks until disconnect, we are good.
	}()

	return nil
}

// Disconnect stops the VPN connection
func (a *App) Disconnect() {
	if a.cancelFunc != nil {
		a.cancelFunc()
		a.cancelFunc = nil
		runtime.EventsEmit(a.ctx, "status", "Disconnecting...")
	}
}

// BeforeClose is called when the application is about to close
func (a *App) BeforeClose(ctx context.Context) (prevent bool) {
	a.Disconnect()
	a.wg.Wait()
	return false
}
