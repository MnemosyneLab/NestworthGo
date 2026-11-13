//go:build darwin && cgo && !server

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation
#import <Foundation/Foundation.h>
#include <stdlib.h>
#include <string.h>

static char *preferredLanguage(void) {
	@autoreleasepool {
		const char *language = [[[NSLocale preferredLanguages] firstObject] UTF8String];
		return language ? strdup(language) : NULL;
	}
}
*/
import "C"

import "unsafe"

func systemLocale() string {
	language := C.preferredLanguage()
	if language == nil {
		return environmentLocale()
	}
	defer C.free(unsafe.Pointer(language))
	return C.GoString(language)
}
