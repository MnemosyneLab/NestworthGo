//go:build darwin && !ios

#import <AppKit/AppKit.h>
#import <objc/runtime.h>
#import "_cgo_export.h"

@class NestworthAXBridge;

@interface NestworthAXElement : NSAccessibilityElement
@property(nonatomic, weak) NestworthAXBridge *bridge;
@property(nonatomic, weak) id parent;
@property(nonatomic, copy) NSArray<NestworthAXElement *> *children;
@property(nonatomic, copy) NSDictionary *entry;
- (id)hitTest:(NSPoint)point;
@end

@interface NestworthAXBridge : NSObject
@property(nonatomic) uintptr_t handle;
@property(nonatomic, weak) NSView *view;
@property(nonatomic) Class originalClass;
@property(nonatomic, copy) NSArray<NestworthAXElement *> *children;
@property(nonatomic, copy) NSDictionary<NSNumber *, NestworthAXElement *> *elements;
@property(nonatomic, weak) NestworthAXElement *focused;
@property(nonatomic) BOOL observing;
- (void)publish:(NSArray<NSDictionary *> *)entries;
@end

// Must match Role and action in frame.go.
typedef NS_ENUM(NSUInteger, NestworthAXRole) {
	NestworthAXStaticText = 1, NestworthAXGroup, NestworthAXButton, NestworthAXPopup,
	NestworthAXTextField, NestworthAXTextArea, NestworthAXList
};
typedef NS_ENUM(uint8_t, NestworthAXAction) {
	NestworthAXPress = 1, NestworthAXFocus, NestworthAXSetValue, NestworthAXSetSelection
};

static NSNumber *axNumber(NSDictionary *entry, NSString *key) {
	return entry[key];
}

static NSAccessibilityRole axRole(NSUInteger role) {
	switch (role) {
	case NestworthAXStaticText: return NSAccessibilityStaticTextRole;
	case NestworthAXButton: return NSAccessibilityButtonRole;
	case NestworthAXPopup: return NSAccessibilityPopUpButtonRole;
	case NestworthAXTextField: return NSAccessibilityTextFieldRole;
	case NestworthAXTextArea: return NSAccessibilityTextAreaRole;
	case NestworthAXList: return NSAccessibilityListRole;
	default: return NSAccessibilityGroupRole;
	}
}

@implementation NestworthAXElement

- (BOOL)isAccessibilityElement { return YES; }
- (id)accessibilityParent { return self.parent; }
- (NSArray *)accessibilityChildren { return self.children; }
- (NSArray *)accessibilityChildrenInNavigationOrder { return self.children; }
- (NSAccessibilityRole)accessibilityRole { return axRole(axNumber(self.entry, @"Role").unsignedIntegerValue); }
- (NSString *)accessibilityLabel { return self.entry[@"Name"]; }
- (NSString *)accessibilityHelp { return self.entry[@"Help"]; }
- (BOOL)isAccessibilityEnabled { return axNumber(self.entry, @"Enabled").boolValue; }
- (BOOL)isAccessibilitySelected { return axNumber(self.entry, @"Selected").boolValue; }
- (BOOL)isAccessibilityFocused { return self.bridge.focused == self; }

- (id)accessibilityValue {
	NSString *value = self.entry[@"Value"];
	NSUInteger role = axNumber(self.entry, @"Role").unsignedIntegerValue;
	return value.length || role == NestworthAXTextField || role == NestworthAXTextArea ? value : nil;
}

- (NSRect)accessibilityFrame {
	NSView *view = self.bridge.view;
	NSWindow *window = view.window;
	if (!window) {
		return NSZeroRect;
	}
	NSDictionary *entry = self.entry;
	// Gio's coordinates are backing pixels from the upper left of the view.
	NSRect pixels = NSMakeRect(axNumber(entry, @"X").doubleValue, axNumber(entry, @"Y").doubleValue,
		axNumber(entry, @"Width").doubleValue, axNumber(entry, @"Height").doubleValue);
	NSRect rect = [view convertRectFromBacking:pixels];
	if (!view.isFlipped) {
		rect.origin.y = NSMaxY(view.bounds) - NSMaxY(rect);
	}
	return [window convertRectToScreen:[view convertRect:rect toView:nil]];
}

- (id)hitTest:(NSPoint)point {
	if (!NSPointInRect(point, self.accessibilityFrame)) {
		return nil;
	}
	for (NestworthAXElement *child in self.children.reverseObjectEnumerator) {
		id hit = [child hitTest:point];
		if (hit) {
			return hit;
		}
	}
	return self;
}

- (BOOL)send:(NestworthAXAction)action value:(NSString *)value range:(NSRange)range {
	NestworthAXBridge *bridge = self.bridge;
	NSNumber *identifier = self.entry[@"ID"];
	if (!bridge || bridge.elements[identifier] != self || !self.isAccessibilityEnabled) {
		return NO;
	}
	return nestworthAXAction(bridge.handle, identifier.unsignedLongLongValue, action,
		(char *)value.UTF8String, range.location, range.length) != 0;
}

- (BOOL)accessibilityPerformPress {
	return axNumber(self.entry, @"Pressable").boolValue && [self send:NestworthAXPress value:nil range:NSMakeRange(0, 0)];
}

- (void)setAccessibilityFocused:(BOOL)focused {
	if (focused && axNumber(self.entry, @"Focusable").boolValue) {
		[self send:NestworthAXFocus value:nil range:NSMakeRange(0, 0)];
	}
}

- (void)setAccessibilityValue:(id)value {
	if (axNumber(self.entry, @"Editable").boolValue && [value isKindOfClass:NSString.class]) {
		[self send:NestworthAXSetValue value:value range:NSMakeRange(0, 0)];
	}
}

- (NSRange)accessibilitySelectedTextRange {
	return NSMakeRange(axNumber(self.entry, @"SelectionStart").unsignedIntegerValue,
		axNumber(self.entry, @"SelectionLength").unsignedIntegerValue);
}

- (NSString *)accessibilitySelectedText {
	NSString *value = self.entry[@"Value"];
	return [value substringWithRange:self.accessibilitySelectedTextRange];
}

- (NSInteger)accessibilityNumberOfCharacters {
	NSString *value = self.entry[@"Value"];
	return value.length;
}

- (NSString *)accessibilityStringForRange:(NSRange)range {
	NSString *value = self.entry[@"Value"];
	if (range.location > value.length || range.length > value.length - range.location) {
		return nil;
	}
	return [value substringWithRange:range];
}

- (void)setAccessibilitySelectedTextRange:(NSRange)range {
	if (axNumber(self.entry, @"Focusable").boolValue && range.location != NSNotFound) {
		[self send:NestworthAXSetSelection value:nil range:range];
	}
}

- (BOOL)isAccessibilitySelectorAllowed:(SEL)selector {
	if (selector == @selector(accessibilityPerformPress)) {
		return axNumber(self.entry, @"Pressable").boolValue;
	}
	if (selector == @selector(setAccessibilityFocused:)) {
		return axNumber(self.entry, @"Focusable").boolValue;
	}
	if (selector == @selector(setAccessibilityValue:)) {
		return axNumber(self.entry, @"Editable").boolValue;
	}
	if (selector == @selector(accessibilitySelectedTextRange) || selector == @selector(accessibilitySelectedText)
		|| selector == @selector(setAccessibilitySelectedTextRange:)
		|| selector == @selector(accessibilityNumberOfCharacters) || selector == @selector(accessibilityStringForRange:)) {
		NSAccessibilityRole role = self.accessibilityRole;
		BOOL editor = [role isEqualToString:NSAccessibilityTextFieldRole] || [role isEqualToString:NSAccessibilityTextAreaRole];
		return editor && (selector != @selector(setAccessibilitySelectedTextRange:) || self.isAccessibilityEnabled);
	}
	return [super isAccessibilitySelectorAllowed:selector];
}

@end

@implementation NestworthAXBridge

- (void)publish:(NSArray<NSDictionary *> *)entries {
	NSView *view = self.view;
	if (!view.window) {
		return;
	}
	NSMutableDictionary *elements = [NSMutableDictionary dictionaryWithCapacity:entries.count];
	NSMutableDictionary<NSNumber *, NSMutableArray *> *children = [NSMutableDictionary dictionary];
	NSMutableArray<NestworthAXElement *> *valueChanges = [NSMutableArray array];
	NSMutableArray<NestworthAXElement *> *selectionChanges = [NSMutableArray array];
	NestworthAXElement *focused = nil;
	BOOL layoutChanged = self.elements.count != entries.count;
	for (NSDictionary *entry in entries) {
		NSNumber *identifier = entry[@"ID"];
		NSNumber *parent = entry[@"Parent"];
		NestworthAXElement *element = self.elements[identifier];
		if (!element) {
			element = [NestworthAXElement new];
			element.bridge = self;
			layoutChanged = YES;
		} else {
			NSDictionary *previous = element.entry;
			if (![previous[@"Value"] isEqual:entry[@"Value"]]) {
				[valueChanges addObject:element];
			}
			if (![previous[@"SelectionStart"] isEqual:entry[@"SelectionStart"]] ||
				![previous[@"SelectionLength"] isEqual:entry[@"SelectionLength"]]) {
				[selectionChanges addObject:element];
			}
			layoutChanged |= ![previous[@"Parent"] isEqual:parent] ||
				![previous[@"Name"] isEqual:entry[@"Name"]] || ![previous[@"Enabled"] isEqual:entry[@"Enabled"]] ||
				![previous[@"Selected"] isEqual:entry[@"Selected"]];
		}
		element.entry = entry;
		element.parent = parent.unsignedLongLongValue == 0 ? view : elements[parent];
		elements[identifier] = element;
		if (!children[parent]) {
			children[parent] = [NSMutableArray array];
		}
		[children[parent] addObject:element];
		if (axNumber(entry, @"Focused").boolValue) {
			focused = element;
		}
	}
	for (NSNumber *identifier in elements) {
		NestworthAXElement *element = elements[identifier];
		NSArray *next = children[identifier] ?: @[];
		layoutChanged |= ![element.children isEqualToArray:next];
		element.children = next;
	}
	NSArray *root = children[@0] ?: @[];
	layoutChanged |= ![self.children isEqualToArray:root];
	BOOL focusChanged = self.focused != focused;
	self.children = root;
	self.elements = elements;
	self.focused = focused;

	if (layoutChanged) {
		NSAccessibilityPostNotification(view, NSAccessibilityLayoutChangedNotification);
	}
	for (NestworthAXElement *element in valueChanges) {
		NSAccessibilityPostNotification(element, NSAccessibilityValueChangedNotification);
	}
	for (NestworthAXElement *element in selectionChanges) {
		NSAccessibilityPostNotification(element, NSAccessibilitySelectedTextChangedNotification);
	}
	if (focusChanged) {
		NSAccessibilityPostNotification(focused ?: (id)view, NSAccessibilityFocusedUIElementChangedNotification);
	}
}

@end

static const char axBridgeKey;
static NSMutableDictionary<NSNumber *, NestworthAXBridge *> *axBridges;

static NestworthAXBridge *axBridge(NSView *view) {
	return objc_getAssociatedObject(view, &axBridgeKey);
}

static BOOL axIsElement(NSView *view, SEL selector) { return YES; }
static id axViewRole(NSView *view, SEL selector) { return NSAccessibilityGroupRole; }

static id axChildren(NSView *view, SEL selector) {
	NestworthAXBridge *bridge = axBridge(view);
	if (bridge && !bridge.observing) {
		bridge.observing = YES;
		nestworthAXRequest(bridge.handle);
	}
	return bridge.children ?: @[];
}

static id axFocused(NSView *view, SEL selector) {
	return axBridge(view).focused ?: (id)view;
}

static id axHitTest(NSView *view, SEL selector, NSPoint point) {
	NSArray *children = axChildren(view, selector);
	for (NestworthAXElement *child in children.reverseObjectEnumerator) {
		id hit = [child hitTest:point];
		if (hit) {
			return hit;
		}
	}
	return view;
}

void nestworthAXAttach(uintptr_t handle, uintptr_t viewRef) {
	NSView *view = (__bridge NSView *)(void *)viewRef;
	dispatch_async(dispatch_get_main_queue(), ^{
		Class base = object_getClass(view);
		Class provider = objc_lookUpClass("NestworthGioAccessibleView");
		if (!provider) {
			// Install the provider on this instance only. Gio keeps its usual
			// rendering, input and window lifetime methods through inheritance.
			provider = objc_allocateClassPair(base, "NestworthGioAccessibleView", 0);
			if (!provider) {
				return;
			}
			class_addMethod(provider, @selector(isAccessibilityElement), (IMP)axIsElement, "B@:");
			class_addMethod(provider, @selector(accessibilityRole), (IMP)axViewRole, "@@:");
			class_addMethod(provider, @selector(accessibilityChildren), (IMP)axChildren, "@@:");
			class_addMethod(provider, @selector(accessibilityChildrenInNavigationOrder), (IMP)axChildren, "@@:");
			class_addMethod(provider, @selector(accessibilityFocusedUIElement), (IMP)axFocused, "@@:");
			Method hit = class_getInstanceMethod(base, @selector(accessibilityHitTest:));
			if (hit) {
				class_addMethod(provider, @selector(accessibilityHitTest:), (IMP)axHitTest, method_getTypeEncoding(hit));
			}
			objc_registerClassPair(provider);
		}
		if (class_getSuperclass(provider) != base) {
			return;
		}
		if (!axBridges) {
			axBridges = [NSMutableDictionary dictionary];
		}
		NestworthAXBridge *bridge = [NestworthAXBridge new];
		bridge.handle = handle;
		bridge.view = view;
		bridge.originalClass = base;
		axBridges[@(handle)] = bridge;
		objc_setAssociatedObject(view, &axBridgeKey, bridge, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
		object_setClass(view, provider);
		// A client may have inspected Gio's original view before attachment.
		NSAccessibilityPostNotification(view, NSAccessibilityLayoutChangedNotification);
	});
}

void nestworthAXPublish(uintptr_t handle, const void *bytes, size_t length) {
	@autoreleasepool {
		NSData *data = [NSData dataWithBytes:bytes length:length];
		dispatch_async(dispatch_get_main_queue(), ^{
			id decoded = [NSJSONSerialization JSONObjectWithData:data options:0 error:nil];
			if ([decoded isKindOfClass:NSArray.class]) {
				[axBridges[@(handle)] publish:decoded];
			}
		});
	}
}

void nestworthAXDetach(uintptr_t handle) {
	dispatch_async(dispatch_get_main_queue(), ^{
		NestworthAXBridge *bridge = axBridges[@(handle)];
		NSView *view = bridge.view;
		if (view) {
			object_setClass(view, bridge.originalClass);
			objc_setAssociatedObject(view, &axBridgeKey, nil, OBJC_ASSOCIATION_RETAIN_NONATOMIC);
		}
		[axBridges removeObjectForKey:@(handle)];
	});
}
