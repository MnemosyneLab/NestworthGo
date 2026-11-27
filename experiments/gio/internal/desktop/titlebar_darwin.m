//go:build darwin && !ios && cgo

#import <AppKit/AppKit.h>
#import <QuartzCore/QuartzCore.h>
#include <math.h>
#import "titlebar_darwin.h"

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
@property (strong) NSToolbar *originalToolbar;
@property (strong) NSToolbar *integratedToolbar;
@property NSWindowToolbarStyle originalToolbarStyle;
@property NSTitlebarSeparatorStyle originalSeparator;
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

// Measure actual, visible standard AppKit controls. An object returned by
// standardWindowButton is not sufficient if Gio or AppKit hides it.
static NSUInteger visibleButtons(NSView *view, NSWindow *window, CGFloat *right, CGFloat *bottom, CGFloat *center) {
    NSUInteger visible = 0;
    CGFloat r = 0, b = 0, centers = 0;
    for (NSButton *button in windowButtons(window)) {
        if (!button.superview || button.window != window || button.alphaValue <= 0 ||
            [button isHiddenOrHasHiddenAncestor]) continue;
        NSRect bounds = topLeftRect(view, [view convertRect:[button convertRect:button.bounds toView:nil] fromView:nil]);
        if (!(window.styleMask & NSWindowStyleMaskFullScreen) && !NSContainsRect(view.bounds, bounds))
            continue;
        bounds = NSIntersectionRect(bounds, view.bounds);
        if (!NSIsEmptyRect(bounds)) {
            visible++;
            r = MAX(r, NSMaxX(bounds));
            b = MAX(b, NSMaxY(bounds));
            centers += NSMidY(bounds);
        }
    }
    if (right) *right = r;
    if (bottom) *bottom = b;
    if (center) *center = visible ? centers / visible : 0;
    return visible;
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
    window.toolbar = self.originalToolbar;
    window.toolbarStyle = self.originalToolbarStyle;
    self.integratedToolbar = nil;
    self.originalToolbar = nil;
    window.styleMask &= ~NSWindowStyleMaskFullSizeContentView;
    window.titleVisibility = NSWindowTitleVisible;
    window.titlebarAppearsTransparent = NO;
    window.titlebarSeparatorStyle = self.originalSeparator;
    window.appearance = self.originalAppearance;
    if (!(window.styleMask & NSWindowStyleMaskFullScreen)) {
        for (NSButton *button in windowButtons(window)) button.hidden = NO;
    }
    self.view.layer.frame = self.view.bounds;
    [self.view setNeedsDisplay:YES];
}

- (void)refresh {
    if (self.detached) return;
    NSView *view = self.view;
    NSWindow *window = self.window;
    if (!window || view.window != window || window.contentView != view) {
        self.unavailable = YES;
        nestworthTitlebarChanged(self.token, NestworthTitlebarUnavailable, 0, 0, 0, 0, 0, 0);
        return;
    }
    if (self.transition) return; // AppKit owns the full-screen animation.
    BOOL fullscreen = (window.styleMask & NSWindowStyleMaskFullScreen) != 0;
    NSArray<NSButton *> *buttons = windowButtons(window);
    if (self.unavailable || (!fullscreen && (buttons.count != 3 ||
        !(window.styleMask & NSWindowStyleMaskTitled)))) {
        self.unavailable = YES;
        [self restore];
        nestworthTitlebarChanged(self.token, NestworthTitlebarUnavailable, 0, 0, 0, 0, 0, 0);
        return;
    }
    if (!self.enabled) {
        [self restore];
        // Do not touch the initial system titlebar before Gio releases
        // ownership of its fallback decoration. Go acts on this preflight.
        nestworthTitlebarChanged(self.token, NestworthTitlebarReady, 0, 0, 0, 0, 0, 0);
        return;
    }
    BOOL firstInstall = !self.installed;
    if (firstInstall) {
        self.originalAppearance = window.appearance;
        self.originalToolbar = window.toolbar;
        self.originalToolbarStyle = window.toolbarStyle;
        self.originalSeparator = window.titlebarSeparatorStyle;
        // Let AppKit provide the taller titlebar and position its own buttons.
        // Gio draws the navigation controls in the full-size content view.
        self.integratedToolbar = [[NSToolbar alloc] initWithIdentifier:@"NestworthGioTitlebar"];
        self.installed = YES;
    }
    // Go has now requested Decorated(false) using Gio's public API. The
    // backend reports Decorated=false for full-size content regardless of
    // button visibility, and Configure hides all three buttons. Restore the
    // REAL AppKit controls here, without requesting Gio's fallback bar.
    BOOL changed = !(window.styleMask & NSWindowStyleMaskFullSizeContentView) ||
        !window.titlebarAppearsTransparent || window.titleVisibility != NSWindowTitleHidden;
    if (window.toolbar != self.integratedToolbar) {
        window.toolbarStyle = NSWindowToolbarStyleUnified;
        window.toolbar = self.integratedToolbar;
        changed = YES;
    }
    if (!(window.styleMask & NSWindowStyleMaskFullSizeContentView))
        window.styleMask |= NSWindowStyleMaskFullSizeContentView;
    if (!window.titlebarAppearsTransparent) window.titlebarAppearsTransparent = YES;
    if (window.titleVisibility != NSWindowTitleHidden) window.titleVisibility = NSWindowTitleHidden;
    if (window.titlebarSeparatorStyle != NSTitlebarSeparatorStyleNone)
        window.titlebarSeparatorStyle = NSTitlebarSeparatorStyleNone;
    NSAppearanceName name = self.dark ? NSAppearanceNameDarkAqua : NSAppearanceNameAqua;
    if (![window.appearance.name isEqualToString:name])
        window.appearance = [NSAppearance appearanceNamed:name];
    buttons = windowButtons(window);
    if (!fullscreen) {
        for (NSButton *button in buttons) {
            if (button.hidden) {
                button.hidden = NO;
                changed = YES;
            }
        }
    }
    if (firstInstall && !fullscreen && !window.isZoomed && self.frameSize.width > 0 &&
        self.frameSize.height > 0 && !NSEqualSizes(view.bounds.size, self.frameSize)) {
        // The size is the last Gio frame BEFORE Decorated(false), not the
        // already-expanded NSView. Preserve that content size across startup
        // so repeated launches do not grow by one native titlebar height.
        [window setContentSize:self.frameSize];
        changed = YES;
    }
    if (changed || firstInstall) {
        // The style change can move AppKit's titlebar controls. Measure
        // their actual laid-out frames, not the pre-change geometry.
        [view.superview layoutSubtreeIfNeeded];
        view.layer.frame = view.bounds;
        [view setNeedsDisplay:YES];
    }
    NSEdgeInsets safe = view.safeAreaInsets;
    CGFloat right = 0, bottom = 0, center = 0;
    NSUInteger visible = visibleButtons(view, window, &right, &bottom, &center);
    // Every normal-window native control must really intersect the full
    // content view. If not, restore native decorations rather than putting
    // Gio controls on top of unknown button geometry.
    if (!fullscreen && (visible != 3 || right <= 0 || bottom <= 0 ||
        right > NSWidth(view.bounds)/2 || bottom > NSHeight(view.bounds)/4)) {
        self.unavailable = YES;
        [self restore];
        nestworthTitlebarChanged(self.token, NestworthTitlebarUnavailable, 0, 0, 0, 0, 0, 0);
        return;
    }
    nestworthTitlebarChanged(self.token, NestworthTitlebarIntegrated, ceil(right), ceil(bottom), center,
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
        if (!window || window.contentView != view) {
            nestworthTitlebarChanged(token, NestworthTitlebarUnavailable, 0, 0, 0, 0, 0, 0);
            return;
        }
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
        [state refresh];
    });
}

void nestworthTitlebarRefresh(uintptr_t token) {
    dispatch_async(dispatch_get_main_queue(), ^{ [titlebars[@(token)] refresh]; });
}

// These two functions must be called through Gio's public Window.Run on the
// native event thread. No dispatch_sync (which would deadlock during events).
void nestworthTitlebarSetMode(uintptr_t token, int enabled, int dark, double width, double height) {
    NestworthTitlebar *s = titlebars[@(token)];
    if (!s) {
        nestworthTitlebarChanged(token, NestworthTitlebarUnavailable, 0, 0, 0, 0, 0, 0);
        return;
    }
    s.enabled = enabled != 0;
    s.dark = dark != 0;
    s.frameSize = NSMakeSize(width, height);
    s.dragA = s.dragB = NSZeroRect;
    [s refresh];
}

NestworthTitlebarProbe nestworthTitlebarProbe(uintptr_t token) {
    NestworthTitlebarProbe result = {0};
    NestworthTitlebar *s = titlebars[@(token)];
    NSWindow *window = s.window;
    NSView *view = s.view;
    if (!window || view.window != window || window.contentView != view) return result;
    NSSize pixels = [view convertSizeToBacking:view.bounds.size];
    result.widthPx = (int)llround(pixels.width);
    result.heightPx = (int)llround(pixels.height);
    result.outerWidth = NSWidth(window.frame);
    result.outerHeight = NSHeight(window.frame);
    result.fullSize = (window.styleMask & NSWindowStyleMaskFullSizeContentView) != 0;
    result.fullscreen = (window.styleMask & NSWindowStyleMaskFullScreen) != 0;
    result.transition = s.transition;
    result.visibleButtons = (int)visibleButtons(view, window, NULL, NULL, NULL);
    return result;
}

void nestworthTitlebarUpdate(uintptr_t token, int dark, double width, double height,
                             double x1, double y1, double w1, double h1,
                             double x2, double y2, double w2, double h2) {
    dispatch_async(dispatch_get_main_queue(), ^{
        NestworthTitlebar *s = titlebars[@(token)];
        if (!s.enabled || s.detached) return;
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
