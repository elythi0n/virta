//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework WebKit

#import <Cocoa/Cocoa.h>
#import <WebKit/WebKit.h>

// OverlayDelegate responds to WKWebView navigation and window events.
@interface OverlayDelegate : NSObject <NSWindowDelegate>
@end

@implementation OverlayDelegate
- (BOOL)windowShouldClose:(NSWindow *)sender {
    [NSApp terminate:nil];
    return YES;
}
@end

static void runOverlayObjC(const char *urlStr, const char *titleStr,
                            int x, int y, int width, int height) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];

        // Build the window content rect anchored at (x, y) in screen coordinates.
        // Cocoa uses bottom-left origin; the rect is relative to the screen.
        NSRect contentRect = NSMakeRect((CGFloat)x, (CGFloat)y,
                                        (CGFloat)width, (CGFloat)height);

        NSWindowStyleMask styleMask = NSWindowStyleMaskBorderless;

        NSWindow *window = [[NSWindow alloc]
            initWithContentRect:contentRect
                      styleMask:styleMask
                        backing:NSBackingStoreBuffered
                          defer:NO];

        // Make the window transparent and floating.
        [window setOpaque:NO];
        [window setBackgroundColor:[NSColor clearColor]];
        [window setLevel:NSFloatingWindowLevel];
        [window setCollectionBehavior:
            NSWindowCollectionBehaviorCanJoinAllSpaces |
            NSWindowCollectionBehaviorStationary       |
            NSWindowCollectionBehaviorIgnoresCycle];

        // Click-through: pointer events are not delivered to this window.
        [window setIgnoresMouseEvents:YES];

        // Set the window title so OBS can find it.
        [window setTitle:[NSString stringWithUTF8String:titleStr]];

        // Delegate handles window-close.
        OverlayDelegate *delegate = [[OverlayDelegate alloc] init];
        [window setDelegate:delegate];

        // Build the WKWebView configuration with transparent background.
        WKWebViewConfiguration *config = [[WKWebViewConfiguration alloc] init];

        WKWebView *webView = [[WKWebView alloc]
            initWithFrame:[[window contentView] bounds]
            configuration:config];

        // Disable the default white background drawn by WKWebView.
        [webView setValue:@NO forKey:@"drawsBackground"];
        [webView setAutoresizingMask:NSViewWidthSizable | NSViewHeightSizable];

        [window setContentView:webView];

        // Load the overlay URL.
        NSURL *url = [NSURL URLWithString:[NSString stringWithUTF8String:urlStr]];
        NSURLRequest *request = [NSURLRequest requestWithURL:url];
        [webView loadRequest:request];

        [window makeKeyAndOrderFront:nil];

        [NSApp run];
    }
}
*/
import "C"

import (
	"fmt"
	"unsafe"
)

func runOverlay(urlStr, title string, x, y, width, height int) error {
	if urlStr == "" {
		return fmt.Errorf("URL must not be empty")
	}
	cURL := C.CString(urlStr)
	cTitle := C.CString(title)
	defer C.free(unsafe.Pointer(cURL))
	defer C.free(unsafe.Pointer(cTitle))
	C.runOverlayObjC(cURL, cTitle, C.int(x), C.int(y), C.int(width), C.int(height))
	return nil
}
