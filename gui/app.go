package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"runtime/debug"

	"github.com/kayrus/gof5/pkg/client"
	"github.com/kayrus/gof5/pkg/config"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx        context.Context
	cancelFunc context.CancelFunc
	wg         sync.WaitGroup
	mu         sync.Mutex
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
	a.mu.Lock()
	if a.cancelFunc != nil {
		a.mu.Unlock()
		return fmt.Errorf("already connected")
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.cancelFunc = cancel
	a.mu.Unlock()

	opts := client.Options{
		Server:   server,
		Username: username,
		Password: password,
		Debug:    false,
	}
	if v := os.Getenv("GOF5_COOKIE_KEY"); v != "" {
		opts.CookieKey = v
	} else {
		opts.NoStoreCookies = true
		log.Printf("Cookie persistence disabled; set GOF5_COOKIE_KEY to enable encryption")
	}
	if cfg, err := config.ReadConfig(false); err == nil && cfg.InsecureTLS {
		log.Printf("Warning: insecureTLS is set in config but is ignored; GUI enforces secure TLS")
	}

	a.wg.Add(1)
	go func() {
		defer a.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				err := fmt.Errorf("panic detected: %v", r)
				stack := string(debug.Stack())
				log.Printf("%s\n%s", err, stack)
				runtime.EventsEmit(a.ctx, "error", err.Error())
				runtime.EventsEmit(a.ctx, "status", "Error")
			}
		}()
		defer func() {
			a.mu.Lock()
			a.cancelFunc = nil
			a.mu.Unlock()
		}()

		retryCount := 0
		for {
			runtime.EventsEmit(a.ctx, "status", "Connecting...")
			startTime := time.Now()
			err := client.Connect(ctx, &opts)

			// Check if we should reset retry count (connection lasted > 1 minute)
			if time.Since(startTime) > 1*time.Minute {
				retryCount = 0
			}

			// Common failure handling logic
			if err != nil {
				runtime.EventsEmit(a.ctx, "error", err.Error())
				runtime.EventsEmit(a.ctx, "status", "Error")
			} else {
				runtime.EventsEmit(a.ctx, "status", "Disconnected")
			}

			// Check cancellation
			if ctx.Err() == context.Canceled {
				runtime.EventsEmit(a.ctx, "status", "Disconnected")
				runtime.EventsEmit(a.ctx, "log", "VPN session ended.\n")
				return
			}

			retryCount++
			if retryCount > 5 {
				runtime.EventsEmit(a.ctx, "status", "Disconnected")
				runtime.EventsEmit(a.ctx, "log", "Max retry attempts exceeded. Stopping.\n")
				// Ensure we clean up cancelFunc so user can click Connect again
				a.mu.Lock()
				if a.cancelFunc != nil {
					a.cancelFunc() // Cancel context to be safe
					a.cancelFunc = nil
				}
				a.mu.Unlock()
				return
			}

			// Backoff: 3 * retryCount
			waitDuration := time.Duration(3*retryCount) * time.Second
			msg := fmt.Sprintf("Connection failed/ended. Retrying in %s (Attempt %d/5)....\n", waitDuration, retryCount)
			if err == nil {
				msg = fmt.Sprintf("VPN session ended unexpected. Retrying in %s (Attempt %d/5)....\n", waitDuration, retryCount)
			}
			runtime.EventsEmit(a.ctx, "log", msg)

			select {
			case <-ctx.Done():
				runtime.EventsEmit(a.ctx, "status", "Disconnected")
				runtime.EventsEmit(a.ctx, "log", "Retry cancelled.\n")
				return
			case <-time.After(waitDuration):
				continue
			}
		}
	}()

	return nil
}

// Disconnect stops the VPN connection
func (a *App) Disconnect() {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancelFunc != nil {
		a.cancelFunc()
		// a.cancelFunc = nil // This is now handled in the goroutine defer
		runtime.EventsEmit(a.ctx, "status", "Disconnecting...")
	}
}

// BeforeClose is called when the application is about to close
func (a *App) BeforeClose(ctx context.Context) (prevent bool) {
	a.Disconnect()
	a.wg.Wait()
	return false
}
