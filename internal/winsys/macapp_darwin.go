//go:build darwin && cgo

// The macOS app: a menu bar icon and the settings window as a window of its own (WebKit), so no browser is needed.
// Cocoa only runs on the main thread, so the program's work runs beside it (RunMain). Only the macOS build contains
// this file; without cgo (cross-compiling), tray_other.go and the browser window are used instead.
package winsys

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -mmacosx-version-min=11.0
#cgo LDFLAGS: -framework Cocoa -framework WebKit -mmacosx-version-min=11.0
#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>
#include <stdatomic.h>
#include <stdlib.h>

static NSStatusItem *item;
static NSMenuItem *statusLine, *toggleItem;
static NSWindow *window;
static WKWebView *web;
static atomic_int pending; // the menu command waiting for the Go side: 1 open, 2 start/stop, 3 quit

@interface MRController : NSObject <NSApplicationDelegate, NSWindowDelegate, WKUIDelegate, WKNavigationDelegate>
@end

@implementation MRController
- (void)act:(NSMenuItem *)sender { atomic_store(&pending, (int)sender.tag); }

// The app is opened again (Finder, Dock, Spotlight) while it runs: macOS does not start a second copy but tells this
// one, which shows its window.
- (BOOL)applicationShouldHandleReopen:(NSApplication *)app hasVisibleWindows:(BOOL)visible {
	atomic_store(&pending, 1);
	return NO;
}

- (void)windowWillClose:(NSNotification *)n { window = nil; web = nil; }

// alert() and confirm() from the page (WebKit shows nothing unless the app does)
- (void)webView:(WKWebView *)v runJavaScriptAlertPanelWithMessage:(NSString *)message initiatedByFrame:(WKFrameInfo *)frame
	completionHandler:(void (^)(void))done {
	NSAlert *a = [NSAlert new];
	a.messageText = message;
	[a beginSheetModalForWindow:window completionHandler:^(NSModalResponse r) { done(); }];
}
- (void)webView:(WKWebView *)v runJavaScriptConfirmPanelWithMessage:(NSString *)message initiatedByFrame:(WKFrameInfo *)frame
	completionHandler:(void (^)(BOOL))done {
	NSAlert *a = [NSAlert new];
	a.messageText = message;
	[a addButtonWithTitle:@"OK"];
	[a addButtonWithTitle:@"Cancel"];
	[a beginSheetModalForWindow:window completionHandler:^(NSModalResponse r) { done(r == NSAlertFirstButtonReturn); }];
}
// window.open (the release page): the user's browser
- (WKWebView *)webView:(WKWebView *)v createWebViewWithConfiguration:(WKWebViewConfiguration *)c
	forNavigationAction:(WKNavigationAction *)a windowFeatures:(WKWindowFeatures *)f {
	if (a.request.URL) [[NSWorkspace sharedWorkspace] openURL:a.request.URL];
	return nil;
}
// window.close() from the page (the program has quit)
- (void)webViewDidClose:(WKWebView *)v { [window close]; }
// the window only shows the program's own page; any other link opens in the browser
- (void)webView:(WKWebView *)v decidePolicyForNavigationAction:(WKNavigationAction *)a
	decisionHandler:(void (^)(WKNavigationActionPolicy))decide {
	NSURL *u = a.request.URL;
	if ([u.host isEqualToString:@"127.0.0.1"] || [u.scheme isEqualToString:@"about"]) {
		decide(WKNavigationActionPolicyAllow);
		return;
	}
	[[NSWorkspace sharedWorkspace] openURL:u];
	decide(WKNavigationActionPolicyCancel);
}
@end

static MRController *controller;

// A menu that is never shown (the app has no menu bar of its own) but gives the window copy, paste and Cmd+W.
static void mainMenu(void) {
	NSMenu *bar = [NSMenu new];
	NSMenuItem *appItem = [bar addItemWithTitle:@"" action:nil keyEquivalent:@""];
	NSMenu *app = [NSMenu new];
	[app addItemWithTitle:@"Close Window" action:@selector(performClose:) keyEquivalent:@"w"];
	appItem.submenu = app;
	NSMenuItem *editItem = [bar addItemWithTitle:@"" action:nil keyEquivalent:@""];
	NSMenu *edit = [[NSMenu alloc] initWithTitle:@"Edit"];
	[edit addItemWithTitle:@"Undo" action:NSSelectorFromString(@"undo:") keyEquivalent:@"z"];
	[edit addItemWithTitle:@"Cut" action:@selector(cut:) keyEquivalent:@"x"];
	[edit addItemWithTitle:@"Copy" action:@selector(copy:) keyEquivalent:@"c"];
	[edit addItemWithTitle:@"Paste" action:@selector(paste:) keyEquivalent:@"v"];
	[edit addItemWithTitle:@"Select All" action:@selector(selectAll:) keyEquivalent:@"a"];
	editItem.submenu = edit;
	NSApp.mainMenu = bar;
}

static void macRunApp(void) {
	@autoreleasepool {
		[NSApplication sharedApplication];
		[NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
		controller = [MRController new];
		NSApp.delegate = controller;
		mainMenu();
		[NSApp run];
	}
}

static NSMenuItem *menuItem(NSMenu *m, NSString *title, NSString *key, int tag) {
	NSMenuItem *i = [m addItemWithTitle:title action:@selector(act:) keyEquivalent:key];
	i.target = controller;
	i.tag = tag;
	return i;
}

static void macTrayCreate(const void *png, int len, const char *tip) {
	NSData *data = [NSData dataWithBytes:png length:len];
	NSString *text = [NSString stringWithUTF8String:tip];
	dispatch_async(dispatch_get_main_queue(), ^{
		item = [[NSStatusBar systemStatusBar] statusItemWithLength:NSSquareStatusItemLength];
		NSImage *img = [[NSImage alloc] initWithData:data];
		img.size = NSMakeSize(18, 18);
		img.template = YES; // drawn in the menu bar's own colour, light or dark
		item.button.image = img;
		item.button.toolTip = text;
		NSMenu *m = [NSMenu new];
		m.autoenablesItems = NO;
		statusLine = [m addItemWithTitle:text action:nil keyEquivalent:@""];
		statusLine.enabled = NO;
		[m addItem:[NSMenuItem separatorItem]];
		menuItem(m, @"Open settings", @",", 1);
		toggleItem = menuItem(m, @"Stop presence", @"", 2);
		[m addItem:[NSMenuItem separatorItem]];
		menuItem(m, @"Quit MPCvibedRPC", @"q", 3);
		item.menu = m;
	});
}

static void macTraySetState(const char *tip, int running) {
	NSString *text = [NSString stringWithUTF8String:tip];
	dispatch_async(dispatch_get_main_queue(), ^{
		if (!item) return;
		item.button.toolTip = text;
		statusLine.title = text;
		toggleItem.title = running ? @"Stop presence" : @"Start presence";
	});
}

static int macTrayTakeAction(void) { return atomic_exchange(&pending, 0); }

static void macTrayRemove(void) {
	dispatch_semaphore_t done = dispatch_semaphore_create(0);
	dispatch_async(dispatch_get_main_queue(), ^{
		if (item) [[NSStatusBar systemStatusBar] removeStatusItem:item];
		item = nil;
		dispatch_semaphore_signal(done);
	});
	dispatch_semaphore_wait(done, dispatch_time(DISPATCH_TIME_NOW, 1500 * NSEC_PER_MSEC));
}

static void macShowWindow(const char *url) {
	NSURL *u = [NSURL URLWithString:[NSString stringWithUTF8String:url]];
	dispatch_async(dispatch_get_main_queue(), ^{
		if (!window) {
			window = [[NSWindow alloc] initWithContentRect:NSMakeRect(0, 0, 620, 700)
				styleMask:NSWindowStyleMaskTitled | NSWindowStyleMaskClosable | NSWindowStyleMaskMiniaturizable | NSWindowStyleMaskResizable
				backing:NSBackingStoreBuffered defer:NO];
			window.releasedWhenClosed = NO;
			window.title = @"MPCvibedRPC";
			window.delegate = controller;
			window.minSize = NSMakeSize(420, 400);
			[window center];
			[window setFrameAutosaveName:@"Settings"];
			web = [[WKWebView alloc] initWithFrame:window.contentView.bounds configuration:[WKWebViewConfiguration new]];
			web.autoresizingMask = NSViewWidthSizable | NSViewHeightSizable;
			web.UIDelegate = controller;
			web.navigationDelegate = controller;
			window.contentView = web;
			[web loadRequest:[NSURLRequest requestWithURL:u]];
		}
		[window makeKeyAndOrderFront:nil];
		[window orderFrontRegardless]; // also when another app is active (opening the app again from the Finder)
		[NSApp activateIgnoringOtherApps:YES];
	});
}

static void macCloseWindow(void) {
	dispatch_async(dispatch_get_main_queue(), ^{ [window close]; });
}
*/
import "C"

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"runtime"
	"sync"
	"time"
	"unsafe"
)

// Cocoa wants the main thread: keep the main goroutine on it until RunMain hands it over.
func init() { runtime.LockOSThread() }

// macApp is true where this file is built: the menu bar icon and the window of its own exist.
const macApp = true

// RunMain runs the program (f) beside the Cocoa event loop, which takes over the main thread; the process ends when
// f returns.
func RunMain(f func()) {
	go func() {
		f()
		os.Exit(0)
	}()
	C.macRunApp()
}

// openNativeWindow shows the settings page in the app's own window.
func openNativeWindow(url string) bool {
	c := C.CString(url)
	defer C.free(unsafe.Pointer(c))
	C.macShowWindow(c)
	return true
}

// closeNativeWindow closes that window (when the program quits).
func closeNativeWindow() { C.macCloseWindow() }

// TrayOptions describes the menu bar icon.
type TrayOptions struct {
	Icon     []byte
	Dir      string
	Tooltip  func() string
	Running  func() bool
	OnOpen   func()
	OnToggle func()
	OnQuit   func()
	Log      func(level, msg string)
}

// Tray is the menu bar icon.
type Tray struct {
	o    TrayOptions
	stop chan struct{}
	once sync.Once
}

// StartTray puts the icon in the menu bar. Its menu shows the status, Open settings, Start/Stop presence and Quit.
func StartTray(o TrayOptions) *Tray {
	t := &Tray{o: o, stop: make(chan struct{})}
	icon := menuBarIcon(36)
	p := C.CBytes(icon)
	defer C.free(p)
	tip, running := t.state()
	c := C.CString(tip)
	defer C.free(unsafe.Pointer(c))
	C.macTrayCreate(p, C.int(len(icon)), c)
	t.push(tip, running)
	go t.loop()
	return t
}

func (t *Tray) state() (string, bool) {
	tip, running := "MPCvibedRPC", false
	if t.o.Tooltip != nil {
		tip = t.o.Tooltip()
	}
	if t.o.Running != nil {
		running = t.o.Running()
	}
	return tip, running
}

func (t *Tray) push(tip string, running bool) {
	c := C.CString(tip)
	defer C.free(unsafe.Pointer(c))
	r := C.int(0)
	if running {
		r = 1
	}
	C.macTraySetState(c, r)
}

// loop hands menu commands to the program and keeps the status line current.
func (t *Tray) loop() {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	lastTip, lastRunning, n := "", false, 0
	for {
		select {
		case <-t.stop:
			return
		case <-tick.C:
		}
		var f func()
		switch C.macTrayTakeAction() {
		case 1:
			f = t.o.OnOpen
		case 2:
			f = t.o.OnToggle
		case 3:
			f = t.o.OnQuit
		}
		if f != nil {
			go f()
			n = 9 // show the result soon
		}
		if n++; n >= 10 {
			n = 0
			if tip, running := t.state(); tip != lastTip || running != lastRunning {
				lastTip, lastRunning = tip, running
				t.push(tip, running)
			}
		}
	}
}

// Close removes the icon.
func (t *Tray) Close() {
	if t == nil {
		return
	}
	t.once.Do(func() {
		close(t.stop)
		C.macTrayRemove()
	})
}

// Balloon would show a notification; the menu bar icon has none (the window shows updates).
func (t *Tray) Balloon(title, text string) bool { return false }

// menuBarIcon draws the icon for the menu bar at n x n pixels (18 points on a Retina screen): the program's rounded
// square with the play triangle cut out, in black on transparent, which macOS recolours for light and dark menu bars.
func menuBarIcon(n int) []byte {
	const ss, radius, inset = 8, 0.2, 0.06
	tri := [3][2]float64{{0.39, 0.29}, {0.39, 0.71}, {0.73, 0.5}}
	inRect := func(x, y float64) bool {
		x, y = (x-inset)/(1-2*inset), (y-inset)/(1-2*inset)
		if x < 0 || y < 0 || x > 1 || y > 1 {
			return false
		}
		dx := math.Max(math.Max(radius-x, x-(1-radius)), 0)
		dy := math.Max(math.Max(radius-y, y-(1-radius)), 0)
		return dx*dx+dy*dy <= radius*radius
	}
	inTri := func(x, y float64) bool {
		s := func(a, b [2]float64) float64 { return (x-b[0])*(a[1]-b[1]) - (a[0]-b[0])*(y-b[1]) }
		d1, d2, d3 := s(tri[0], tri[1]), s(tri[1], tri[2]), s(tri[2], tri[0])
		return !((d1 < 0 || d2 < 0 || d3 < 0) && (d1 > 0 || d2 > 0 || d3 > 0))
	}
	img := image.NewNRGBA(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			cover := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					u, v := (float64(x)+(float64(sx)+0.5)/ss)/float64(n), (float64(y)+(float64(sy)+0.5)/ss)/float64(n)
					if inRect(u, v) && !inTri(u, v) {
						cover++
					}
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{0, 0, 0, uint8(math.Round(255 * float64(cover) / (ss * ss)))})
		}
	}
	var out bytes.Buffer
	_ = png.Encode(&out, img)
	return out.Bytes()
}
