//go:build linux

package main

/*
#cgo pkg-config: gtk+-3.0 webkit2gtk-4.1

#include <gtk/gtk.h>
#include <webkit2/webkit2.h>
#include <gdk/gdk.h>
#include <cairo.h>
#include <string.h>
#include <stdlib.h>

// drawCallback paints a fully transparent background so the GTK window has no
// default background that would obstruct the webview transparency.
static gboolean drawCallback(GtkWidget *widget, cairo_t *cr, gpointer data) {
    cairo_set_source_rgba(cr, 0.0, 0.0, 0.0, 0.0);
    cairo_set_operator(cr, CAIRO_OPERATOR_SOURCE);
    cairo_paint(cr);
    return FALSE;
}

// applyInputShape replaces the window's input shape with an empty region so all
// pointer events fall through to whatever is underneath.
static void applyInputShape(GtkWidget *window) {
    cairo_region_t *empty = cairo_region_create();
    gtk_widget_input_shape_combine_region(window, empty);
    cairo_region_destroy(empty);
}

// runOverlayC is the main C entry-point called from Go.
static void runOverlayC(const char *urlStr, const char *titleStr,
                        int x, int y, int width, int height) {
    // Initialise GTK.
    gtk_init(NULL, NULL);

    // Create a top-level window.
    GtkWidget *window = gtk_window_new(GTK_WINDOW_TOPLEVEL);

    // Remove window decorations (title bar, borders).
    gtk_window_set_decorated(GTK_WINDOW(window), FALSE);

    // Keep the window above all others.
    gtk_window_set_keep_above(GTK_WINDOW(window), TRUE);

    // Hide from taskbar and pager.
    gtk_window_set_skip_taskbar_hint(GTK_WINDOW(window), TRUE);
    gtk_window_set_skip_pager_hint(GTK_WINDOW(window), TRUE);

    // Set the window title so OBS can target it.
    gtk_window_set_title(GTK_WINDOW(window), titleStr);

    // Request an RGBA visual so the compositor can composite transparency.
    GdkScreen *screen = gtk_widget_get_screen(window);
    GdkVisual *visual = gdk_screen_get_rgba_visual(screen);
    if (visual != NULL) {
        gtk_widget_set_visual(window, visual);
    }

    // Allow the app to paint directly to the window background.
    gtk_widget_set_app_paintable(window, TRUE);

    // Paint a transparent background on every draw pass.
    g_signal_connect(G_OBJECT(window), "draw", G_CALLBACK(drawCallback), NULL);

    // Quit when the window is closed.
    g_signal_connect(G_OBJECT(window), "destroy", G_CALLBACK(gtk_main_quit), NULL);

    // Create the WebView.
    WebKitWebView *webView = WEBKIT_WEB_VIEW(webkit_web_view_new());

    // Set the WebView background to fully transparent.
    GdkRGBA bg = {0.0, 0.0, 0.0, 0.0};
    webkit_web_view_set_background_color(webView, &bg);

    // Disable context menus so right-click does not surface a dev menu on stream.
    WebKitSettings *settings = webkit_web_view_get_settings(webView);
    webkit_settings_set_enable_developer_extras(settings, FALSE);

    // Place the WebView inside the window.
    gtk_container_add(GTK_CONTAINER(window), GTK_WIDGET(webView));

    // Size and position.
    gtk_window_set_default_size(GTK_WINDOW(window), width, height);
    gtk_window_set_position(GTK_WINDOW(window), GTK_WIN_POS_NONE);
    gtk_window_move(GTK_WINDOW(window), x, y);

    // Show everything so the window and webview are realized before we apply
    // the input shape (the shape must be set after realization).
    gtk_widget_show_all(window);

    // Make the window fully click-through by installing an empty input region.
    applyInputShape(window);

    // Navigate to the overlay URL.
    webkit_web_view_load_uri(webView, urlStr);

    gtk_main();
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
	C.runOverlayC(cURL, cTitle, C.int(x), C.int(y), C.int(width), C.int(height))
	return nil
}
