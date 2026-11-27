//go:build darwin && !ios && cgo

#import <AppKit/AppKit.h>
#import <QuartzCore/QuartzCore.h>
#include <stdint.h>
#include <math.h>

extern void nestworthTitlebarChanged(uintptr_t token, int active, double right, double bottom,
                                    double top, double left, double safeRight);

// All AppKit state belongs to the main queue. No Gio class, delegate, layer
// subclass, standard-button target/action or private view hierarchy is replaced.
@interface NestworthTitlebar : NSObject
@property uintptr_t token;
@property (weak) NSView *view;
@property (weak) NSWindow *window;
@property BOOL enabled, dark, installed, scheduled, transition, detached, unavailable;
@property NSSize frameSize;
@property NSRect dragA, dragB, fillRestore;
@property (strong) NSArray *observers;
@property (strong) id mouseMonitor;
@property (strong) NSAppearance *originalAppearance;
@property NSWindowTitlebarSeparatorStyle originalSeparator;
- (void)refresh;
- (void)schedule;
- (void)restore;
@end

static NSMutableDictionary<NSNumber *, NestworthTitlebar *> *titlebars;

static NSArray<NSButton *> *windowButtons(NSWindow *window) {
    NSButton *close = [window standardWindowButton:NSWindowCloseButton];
    NSButton *minimize = [window standardWindowButton:NSWindowMiniaturizeButton];
    NSButton *zoom = [window standardWindowButton:NSWindowZoomButton];
    return close && minimize && zoom ? @[close, minimize, zoom] : @[];
}

// NSView conversions use points, and Gio's origin is at the upper left.
static NSRect topLeftRect(NSView *view, NSRect rect) {
    if (!view.isFlipped) {
        rect.origin.y = NSHeight(view.bounds) - NSMaxY(rect);
    }
    return rect;
}

@implementation NestworthTitlebar
- (void)schedule {
    if (self.scheduled || self.detached) return;
    self.scheduled = YES;
    dispatch_async(dispatch_get_main_queue(), ^{
        self.scheduled = NO;
        if (!self.detached) [self refresh];
    });
}

- (void)restore {
    NSWindow *window = self.window;
    if (!self.installed || !window || self.view.window != window) return;
    self.installed = NO;
    window.styleMask &= ~NSWindowStyleMaskFullSizeContentView;
    window.titleVisibility = NSWindowTitleVisible;
    window.titlebarAppearsTransparent = NO;
    window.titlebarSeparatorStyle = self.originalSeparator;
    window.appearance = self.originalAppearance;
    if (!(window.styleMask & NSWindowStyleMaskFullScreen)) {
        for (NSButton *button in windowButtons(window)) button.hidden = NO;
    }
    [self.view setNeedsDisplay:YES];
}

- (void)refresh {
    NSView *view = self.view;
    NSWindow *window = self.window;
    if (!window || view.window != window || window.contentView != view) {
        nestworthTitlebarChanged(self.token, 0, 0, 0, 0, 0, 0);
        return;
    }
    if (self.transition) return; // AppKit owns the full-screen animation.
    BOOL fullscreen = (window.styleMask & NSWindowStyleMaskFullScreen) != 0;
    NSArray<NSButton *> *buttons = windowButtons(window);
    if (!self.enabled || self.unavailable || (!fullscreen && buttons.count != 3) ||
        (!fullscreen && !(window.styleMask & NSWindowStyleMaskTitled))) {
        [self restore];
        nestworthTitlebarChanged(self.token, 0, 0, 0, 0, 0, 0);
        return;
    }
    BOOL firstInstall = !self.installed;
    NSSize initialContentSize = view.bounds.size;
    if (firstInstall) {
        self.originalAppearance = window.appearance;
        self.originalSeparator = window.titlebarSeparatorStyle;
        self.installed = YES;
    }
    // Gio v0.10.3 rewrites these properties whenever it configures the window.
    // Keep Decorated(true); reapply idempotently on configuration/resize/update.
    BOOL changed = !(window.styleMask & NSWindowStyleMaskFullSizeContentView) ||
        !window.titlebarAppearsTransparent || window.titleVisibility != NSWindowTitleHidden;
    if (!(window.styleMask & NSWindowStyleMaskFullSizeContentView))
        window.styleMask |= NSWindowStyleMaskFullSizeContentView;
    if (!window.titlebarAppearsTransparent) window.titlebarAppearsTransparent = YES;
    if (window.titleVisibility != NSWindowTitleHidden) window.titleVisibility = NSWindowTitleHidden;
    if (window.titlebarSeparatorStyle != NSWindowTitlebarSeparatorStyleNone)
        window.titlebarSeparatorStyle = NSWindowTitlebarSeparatorStyleNone;
    NSAppearanceName name = self.dark ? NSAppearanceNameDarkAqua : NSAppearanceNameAqua;
    if (![window.appearance.name isEqualToString:name])
        window.appearance = [NSAppearance appearanceNamed:name];
    buttons = windowButtons(window);
    if (!fullscreen) {
        for (NSButton *button in buttons) if (button.hidden) button.hidden = NO;
    }
    if (firstInstall && !fullscreen && !NSEqualSizes(view.bounds.size, initialContentSize)) {
        // Gio persists the size of its content, not the outer frame. Keep the
        // requested content size on first install so repeated launches do not
        // grow by one titlebar. Do not adjust the frame on subsequent refreshes.
        [window setContentSize:initialContentSize];
    }
    if (changed) {
        // Full-size content changes the root view frame. Refresh the public
        // backing layer just as a resize would, without touching Gio internals.
        view.layer.frame = view.bounds;
        [view setNeedsDisplay:YES];
    }
    // The normal titlebar's safe area includes the titlebar itself; consuming
    // that vertical inset would recreate the unwanted strip. In full screen,
    // however, respect the display's safe area and the revealed native controls.
    NSEdgeInsets safe = view.safeAreaInsets;
    CGFloat right = 0, bottom = 0;
    for (NSButton *button in buttons) {
        if (!button.superview || button.window != window || [button isHiddenOrHasHiddenAncestor]) continue;
        NSRect bounds = [view convertRect:[button convertRect:button.bounds toView:nil] fromView:nil];
        bounds = NSIntersectionRect(topLeftRect(view, bounds), view.bounds);
        if (!NSIsEmptyRect(bounds)) {
            right = MAX(right, NSMaxX(bounds));
            bottom = MAX(bottom, NSMaxY(bounds));
        }
    }
    // If AppKit cannot locate the normal controls, retain the safe decorated
    // layout rather than putting a Gio control on top of unknown native ones.
    if (!fullscreen && (right <= 0 || bottom <= 0 || right > NSWidth(view.bounds)/2 || bottom > NSHeight(view.bounds)/4)) {
        self.unavailable = YES;
        [self restore];
        nestworthTitlebarChanged(self.token, 0, 0, 0, 0, 0, 0);
        return;
    }
    nestworthTitlebarChanged(self.token, 1, ceil(right), ceil(bottom),
        fullscreen ? ceil(safe.top) : 0, ceil(safe.left), ceil(safe.right));
}
@end

static void titlebarDoubleClick(NestworthTitlebar *state, NSWindow *window) {
    // Respect the system's Desktop & Dock titlebar double-click preference.
    NSString *action = [[NSUserDefaults standardUserDefaults] stringForKey:@"AppleActionOnDoubleClick"];
    if ([action isEqualToString:@"None"] || [action isEqualToString:@"DoNothing"]) return;
    if ([action isEqualToString:@"Minimize"]) {
        [window performMiniaturize:nil];
    } else if ([action isEqualToString:@"Fill"]) {
        // Use public frame/visibleFrame APIs; keep the pre-fill frame so a
        // second click can restore it. Never use private window tiling APIs.
        NSRect target = window.screen.visibleFrame;
        if (NSIsEmptyRect(target)) return;
        if (NSEqualRects(window.frame, target) && !NSIsEmptyRect(state.fillRestore)) {
            [window setFrame:state.fillRestore display:YES animate:YES];
            state.fillRestore = NSZeroRect;
        } else if (!NSEqualRects(window.frame, target)) {
            state.fillRestore = window.frame;
            [window setFrame:target display:YES animate:YES];
        }
    } else {
        // "Maximize" is the previous spelling of Zoom. Respect the older
        // minimize preference if no modern preference is present.
        if (!action && [[NSUserDefaults standardUserDefaults] boolForKey:@"AppleMiniaturizeOnDoubleClick"])
            [window performMiniaturize:nil];
        else
            [window performZoom:nil];
    }
}

void nestworthTitlebarAttach(uintptr_t token, uintptr_t viewRef) {
    NSView *view = (__bridge NSView *)(void *)viewRef;
    dispatch_async(dispatch_get_main_queue(), ^{
        NSWindow *window = view.window;
        if (!window || window.contentView != view) return;
        if (!titlebars) titlebars = [NSMutableDictionary dictionary];
        NestworthTitlebar *state = [NestworthTitlebar new];
        state.token = token;
        state.view = view;
        state.window = window;
        titlebars[@(token)] = state;

        NSMutableArray *observers = [NSMutableArray array];
        __weak NestworthTitlebar *weakState = state;
        NSNotificationCenter *center = NSNotificationCenter.defaultCenter;
        for (NSNotificationName name in @[NSWindowDidResizeNotification,
             NSWindowDidChangeScreenNotification, NSWindowDidChangeBackingPropertiesNotification,
             NSWindowDidUpdateNotification, NSWindowDidBecomeKeyNotification,
             NSWindowWillEnterFullScreenNotification, NSWindowWillExitFullScreenNotification,
             NSWindowDidEnterFullScreenNotification, NSWindowDidExitFullScreenNotification]) {
            [observers addObject:[center addObserverForName:name object:window queue:nil
                usingBlock:^(NSNotification *note) {
                    NestworthTitlebar *s = weakState;
                    if ([note.name isEqualToString:NSWindowWillEnterFullScreenNotification] ||
                        [note.name isEqualToString:NSWindowWillExitFullScreenNotification]) {
                        s.transition = YES;
                    }
                    if ([note.name isEqualToString:NSWindowDidEnterFullScreenNotification] ||
                        [note.name isEqualToString:NSWindowDidExitFullScreenNotification]) {
                        s.transition = NO;
                    }
                    [s schedule];
                }]];
        }
        state.observers = observers;
        state.mouseMonitor = [NSEvent addLocalMonitorForEventsMatchingMask:NSEventMaskLeftMouseDown
             handler:^NSEvent *(NSEvent *event) {
            NestworthTitlebar *s = weakState;
            NSWindow *w = s.window;
            NSView *v = s.view;
            if (!s.installed || s.transition || !w || event.window != w ||
                !w.isMovable || v.window != w ||
                (w.styleMask & NSWindowStyleMaskFullScreen)) return event;
            // Refuse stale rectangles during a resize or scale change. No
            // native click is consumed unless the actual rendered frame agrees.
            if (fabs(NSWidth(v.bounds)-s.frameSize.width) > 0.75 ||
                fabs(NSHeight(v.bounds)-s.frameSize.height) > 0.75) return event;
            NSPoint p = [v convertPoint:event.locationInWindow fromView:nil];
            if (!v.isFlipped) p.y = NSHeight(v.bounds)-p.y;
            if (!NSPointInRect(p, s.dragA) && !NSPointInRect(p, s.dragB)) return event;
            if (!w.isKeyWindow) [w makeKeyAndOrderFront:nil];
            if (event.clickCount == 2) titlebarDoubleClick(s, w);
            else [w performWindowDragWithEvent:event];
            return nil;
        }];
    });
}

void nestworthTitlebarRefresh(uintptr_t token) {
    dispatch_async(dispatch_get_main_queue(), ^{ [titlebars[@(token)] refresh]; });
}

void nestworthTitlebarUpdate(uintptr_t token, int enabled, int dark, double width, double height,
                             double x1, double y1, double w1, double h1,
                             double x2, double y2, double w2, double h2) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NestworthTitlebar *s = titlebars[@(token)];
        s.enabled = enabled != 0;
        s.dark = dark != 0;
        s.frameSize = NSMakeSize(width, height);
        s.dragA = NSMakeRect(x1, y1, w1, h1);
        s.dragB = NSMakeRect(x2, y2, w2, h2);
        [s refresh];
    });
}

void nestworthTitlebarDetach(uintptr_t token) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NestworthTitlebar *s = titlebars[@(token)];
        s.detached = YES;
        if (s.mouseMonitor) [NSEvent removeMonitor:s.mouseMonitor];
        for (id observer in s.observers) [NSNotificationCenter.defaultCenter removeObserver:observer];
        [s restore];
        [titlebars removeObjectForKey:@(token)];
    });
}
