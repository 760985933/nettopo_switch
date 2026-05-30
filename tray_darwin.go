//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework Cocoa

#import <Cocoa/Cocoa.h>
#import <objc/runtime.h>

extern void goTrayOnClick(int tag);
extern void goSetForceQuit(void);
extern void goTrayReopen(void);

static NSStatusItem* gStatusItem = NULL;

// ── tray menu handler ────────────────────────────────────────────────

@interface _TrayHandler : NSObject
@end

@implementation _TrayHandler
- (void)onClick:(id)sender {
	int tag = (int)[(NSMenuItem*)sender tag];
	goTrayOnClick(tag);
}
@end

static _TrayHandler* gHandler = nil;

// ── NSApplication delegate proxy ─────────────────────────────────────
// Wails sets its own NSApplicationDelegate. We insert a proxy that
// intercepts applicationShouldTerminate: (to distinguish Cmd+Q / Dock
// Quit from a window close button click) and applicationShouldHandleReopen:
// (to restore the window when the Dock icon is clicked).
// All other delegate methods are forwarded to Wails' original delegate.

@interface _TrayAppDelegateProxy : NSObject <NSApplicationDelegate>
@property (nonatomic, weak) id<NSApplicationDelegate> wrapped;
@end

@implementation _TrayAppDelegateProxy

- (NSApplicationTerminateReply)applicationShouldTerminate:(NSApplication *)sender {
	goSetForceQuit();
	if ([self.wrapped respondsToSelector:@selector(applicationShouldTerminate:)]) {
		return [self.wrapped applicationShouldTerminate:sender];
	}
	return NSTerminateNow;
}

- (BOOL)applicationShouldHandleReopen:(NSApplication *)sender hasVisibleWindows:(BOOL)flag {
	goTrayReopen();
	return NO;
}

- (BOOL)respondsToSelector:(SEL)aSelector {
	if (sel_isEqual(aSelector, @selector(applicationShouldTerminate:)) ||
		sel_isEqual(aSelector, @selector(applicationShouldHandleReopen:hasVisibleWindows:))) {
		return YES;
	}
	return [self.wrapped respondsToSelector:aSelector];
}

- (id)forwardingTargetForSelector:(SEL)aSelector {
	return self.wrapped;
}

@end

static _TrayAppDelegateProxy* gAppDelegateProxy = nil;

void tray_install_delegate_proxy(void) {
	void (^block)(void) = ^{
		id<NSApplicationDelegate> original = [NSApp delegate];
		if (original == nil || [original isKindOfClass:[_TrayAppDelegateProxy class]]) {
			return;
		}
		gAppDelegateProxy = [[_TrayAppDelegateProxy alloc] init];
		gAppDelegateProxy.wrapped = original;
		[NSApp setDelegate:gAppDelegateProxy];
	};
	if ([NSThread isMainThread]) {
		block();
	} else {
		dispatch_sync(dispatch_get_main_queue(), block);
	}
}

// ── status item / tray menu ──────────────────────────────────────────

void tray_setup(const char* title) {
	if ([NSThread isMainThread]) {
		gHandler = [[_TrayHandler alloc] init];
		gStatusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
		[gStatusItem.button setTitle:[NSString stringWithUTF8String:title]];
		NSMenu* menu = [[NSMenu alloc] init];
		[menu setAutoenablesItems:NO];
		[gStatusItem setMenu:menu];
	} else {
		dispatch_sync(dispatch_get_main_queue(), ^{
			gHandler = [[_TrayHandler alloc] init];
			gStatusItem = [[NSStatusBar systemStatusBar] statusItemWithLength:NSVariableStatusItemLength];
			[gStatusItem.button setTitle:[NSString stringWithUTF8String:title]];
			NSMenu* menu = [[NSMenu alloc] init];
			[menu setAutoenablesItems:NO];
			[gStatusItem setMenu:menu];
		});
	}
}

void tray_set_title(const char* title) {
	if (gStatusItem) {
		NSString* nsTitle = [NSString stringWithUTF8String:title];
		dispatch_async(dispatch_get_main_queue(), ^{
			gStatusItem.button.title = nsTitle;
		});
	}
}

void tray_add_item(const char* title, int tag) {
	void (^block)(void) = ^{
		NSMenuItem* item = [gStatusItem.menu addItemWithTitle:[NSString stringWithUTF8String:title]
													   action:@selector(onClick:)
												keyEquivalent:@""];
		[item setTag:tag];
		[item setTarget:gHandler];
	};
	if ([NSThread isMainThread]) {
		block();
	} else {
		dispatch_sync(dispatch_get_main_queue(), block);
	}
}

void tray_add_separator() {
	void (^block)(void) = ^{
		[gStatusItem.menu addItem:[NSMenuItem separatorItem]];
	};
	if ([NSThread isMainThread]) {
		block();
	} else {
		dispatch_sync(dispatch_get_main_queue(), block);
	}
}
*/
import "C"
import (
	"time"
	"unsafe"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	trayMenuOpen = 1
	trayMenuQuit = 2
	trayMenuHelp = 3
)

func (a *App) initPlatformTray() {
	// All Cocoa UI must happen on the main thread.
	// Wails calls startup from a goroutine, so we dispatch to the main thread.
	// We add a small delay to ensure the main thread's run loop is active.
	go func() {
		time.Sleep(500 * time.Millisecond)

		// Install delegate proxy before Wails receives any OS quit events.
		// Intercepts Cmd+Q / Dock Quit → sets forceQuit so OnBeforeClose
		// allows the real quit; Dock icon click → restores the hidden window.
		C.tray_install_delegate_proxy()

		title := C.CString("Codex Switch")
		defer C.free(unsafe.Pointer(title))
		C.tray_setup(title)

		openTitle := C.CString("打开面板")
		defer C.free(unsafe.Pointer(openTitle))
		C.tray_add_item(openTitle, C.int(trayMenuOpen))

		helpTitle := C.CString("帮助")
		defer C.free(unsafe.Pointer(helpTitle))
		C.tray_add_item(helpTitle, C.int(trayMenuHelp))

		C.tray_add_separator()

		quitTitle := C.CString("退出")
		defer C.free(unsafe.Pointer(quitTitle))
		C.tray_add_item(quitTitle, C.int(trayMenuQuit))

		// Initial balance fetch
		time.Sleep(1500 * time.Millisecond)
		_ = a.GetUsageBalance("")

		// Periodic balance fetch for tray updates
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for range ticker.C {
			_ = a.GetUsageBalance("")
		}
	}()
}

func (a *App) onBalanceUpdate(balance UsageBalance) {
	if balance.Error == "" && balance.AvailableBalance != "" {
		label := C.CString(balance.AvailableBalance + " " + balance.Currency)
		defer C.free(unsafe.Pointer(label))
		C.tray_set_title(label)
	}
}

func handleTrayClick(tag int) {
	switch tag {
	case trayMenuOpen:
		runtime.WindowShow(mainAppCtx)
		runtime.WindowUnminimise(mainAppCtx)
	case trayMenuQuit:
		forceQuit.Store(true)
		runtime.Quit(mainAppCtx)
	case trayMenuHelp:
		runtime.WindowShow(mainAppCtx)
		runtime.WindowUnminimise(mainAppCtx)
		runtime.EventsEmit(mainAppCtx, "tray:help")
	}
}
