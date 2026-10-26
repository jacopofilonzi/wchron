// Package toast shows the desktop notifications (notify.Toaster) through the
// Wails 3 notifications service (github.com/wailsapp/wails/v3/pkg/services/
// notifications), verified on v3.0.0-beta.29: on Windows it pushes WinRT
// toasts through go-toast/wintoast (falling back to PowerShell), under the
// app name "wchron" with the exe's icon.
//
// The service is not registered with application.RegisterService on purpose:
// Wails stops the whole app when a service's startup fails, and a toast is
// never worth that. Start runs the service startup by hand once the
// application has started; if it fails, Toast only logs. The startup writes
// the toast registration for the current user (HKCU\Software\Classes\
// AppUserModelId\wchron and the activator's CLSID with the exe path), like
// any app showing toasts. ServiceShutdown is not called: it would only save an
// (empty) categories list to the registry.
//
// Clicking a toast calls OnClick (the window opens on the Status page).
package toast

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"sync"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/services/notifications"
)

// Toaster is the notify.Toaster of the app.
type Toaster struct {
	log     *slog.Logger
	onClick func()
	svc     *notifications.NotificationService
	seq     atomic.Int64

	mu    sync.Mutex
	ready bool
}

// New returns a Toaster; nothing is shown before Start.
func New(log *slog.Logger, onClick func()) *Toaster {
	return &Toaster{log: log, onClick: onClick, svc: notifications.New()}
}

// Start initializes the notifications: call it once the application has
// started, on the main thread (application.InvokeSync), where Wails would
// start a registered service (it registers the toast activator with COM).
func (t *Toaster) Start(ctx context.Context) {
	if err := t.svc.ServiceStartup(ctx, application.ServiceOptions{}); err != nil {
		t.log.Error("toast: notifications unavailable", "err", err)
		return
	}
	t.svc.OnNotificationResponse(func(r notifications.NotificationResult) {
		if r.Error != nil {
			t.log.Warn("toast: click", "err", r.Error)
			return
		}
		if r.Response.ActionIdentifier == notifications.DefaultActionIdentifier && t.onClick != nil {
			t.onClick()
		}
	})
	t.mu.Lock()
	t.ready = true
	t.mu.Unlock()
}

// Toast shows a notification.
func (t *Toaster) Toast(title, body string) error {
	t.mu.Lock()
	ready := t.ready
	t.mu.Unlock()
	if !ready {
		return errors.New("notifications not available")
	}
	id := "wchron-" + strconv.FormatInt(t.seq.Add(1), 10)
	if err := t.svc.SendNotification(notifications.NotificationOptions{ID: id, Title: title, Body: body}); err != nil {
		return fmt.Errorf("send notification: %w", err)
	}
	return nil
}
