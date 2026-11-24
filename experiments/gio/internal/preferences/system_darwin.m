//go:build darwin && cgo

#import <Foundation/Foundation.h>
#include <stdint.h>
#include <string.h>

extern void nestworthSystemChanged(uint64_t);

char *nestworthPreferredLocale(void) {
    @autoreleasepool {
        const char *value = [NSLocale.preferredLanguages.firstObject UTF8String];
        return value ? strdup(value) : NULL;
    }
}

char *nestworthSystemTimezone(void) {
    @autoreleasepool {
        const char *value = [NSTimeZone.localTimeZone.name UTF8String];
        return value ? strdup(value) : NULL;
    }
}

int nestworthSystemDark(void) {
    @autoreleasepool {
        return [[NSUserDefaults.standardUserDefaults stringForKey:@"AppleInterfaceStyle"] isEqualToString:@"Dark"];
    }
}

void *nestworthWatchSystem(uint64_t token) {
    @autoreleasepool {
        void (^changed)(NSNotification *) = ^(NSNotification *notification) {
            nestworthSystemChanged(token);
        };
        NSNotificationCenter *local = NSNotificationCenter.defaultCenter;
        NSDistributedNotificationCenter *distributed = NSDistributedNotificationCenter.defaultCenter;
        id appearance = [distributed addObserverForName:@"AppleInterfaceThemeChangedNotification"
                                                  object:nil queue:nil usingBlock:changed];
        id locale = [local addObserverForName:NSCurrentLocaleDidChangeNotification object:nil queue:nil usingBlock:changed];
        id timezone = [local addObserverForName:NSSystemTimeZoneDidChangeNotification object:nil queue:nil usingBlock:changed];
        return [[NSArray alloc] initWithObjects:appearance, locale, timezone, nil];
    }
}

void nestworthUnwatchSystem(void *watch) {
    @autoreleasepool {
        NSArray *tokens = (NSArray *)watch;
        [NSDistributedNotificationCenter.defaultCenter removeObserver:tokens[0]];
        [NSNotificationCenter.defaultCenter removeObserver:tokens[1]];
        [NSNotificationCenter.defaultCenter removeObserver:tokens[2]];
        [tokens release];
    }
}
