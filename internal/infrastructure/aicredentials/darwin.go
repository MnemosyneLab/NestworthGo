//go:build darwin && cgo

package aicredentials

/*
#cgo LDFLAGS: -framework Security -framework CoreFoundation
#include <Security/Security.h>
#include <CoreFoundation/CoreFoundation.h>
#include <stdlib.h>
#include <string.h>

// Verify the running code, not a bundle path supplied by the renderer.
static int nw_trusted(const char *requirement) {
 SecCodeRef code=NULL; SecStaticCodeRef disk=NULL; SecRequirementRef req=NULL; CFDictionaryRef info=NULL;
 CFStringRef text=CFStringCreateWithCString(NULL,requirement,kCFStringEncodingUTF8);
 int ok=0;
 if (!text) return 0;
 if (SecCodeCopySelf(kSecCSDefaultFlags,&code)!=errSecSuccess) goto done;
 if (SecRequirementCreateWithString(text,kSecCSDefaultFlags,&req)!=errSecSuccess) goto done;
 if (SecCodeCheckValidity(code,kSecCSDefaultFlags,req)!=errSecSuccess) goto done;
 if (SecCodeCopyStaticCode(code,kSecCSDefaultFlags,&disk)!=errSecSuccess) goto done;
 if (SecStaticCodeCheckValidity(disk,kSecCSStrictValidate,req)!=errSecSuccess) goto done;
 if (SecCodeCheckValidity(code,kSecCSDefaultFlags,req)!=errSecSuccess) goto done;
 if (SecCodeCopySigningInformation(disk,kSecCSSigningInformation,&info)!=errSecSuccess) goto done;
 CFNumberRef value=(CFNumberRef)CFDictionaryGetValue(info,kSecCodeInfoFlags);
 unsigned int flags=0;
 if (!value || !CFNumberGetValue(value,kCFNumberIntType,&flags)) goto done;
 if (!(flags & kSecCodeSignatureRuntime) || (flags & kSecCodeSignatureAdhoc)) goto done;
 CFDictionaryRef ent=(CFDictionaryRef)CFDictionaryGetValue(info,kSecCodeInfoEntitlementsDict);
 if (ent) {
  const CFStringRef denied[]={CFSTR("com.apple.security.get-task-allow"),CFSTR("com.apple.security.cs.disable-library-validation"),CFSTR("com.apple.security.cs.allow-dyld-environment-variables"),CFSTR("com.apple.security.cs.allow-unsigned-executable-memory")};
  for(int i=0;i<4;i++){if(CFDictionaryGetValue(ent,denied[i])==kCFBooleanTrue)goto done;}
 }
 ok=1;
 done: if(info)CFRelease(info);if(disk)CFRelease(disk);if(req)CFRelease(req);if(code)CFRelease(code);CFRelease(text);return ok;
}
static CFMutableDictionaryRef nw_query(const char *account) {
 CFMutableDictionaryRef q=CFDictionaryCreateMutable(NULL,0,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
 CFStringRef a=CFStringCreateWithCString(NULL,account,kCFStringEncodingUTF8);
 CFDictionarySetValue(q,kSecClass,kSecClassGenericPassword);
 CFDictionarySetValue(q,kSecAttrService,CFSTR("com.nestworth.app.builtin-ai.v1"));
 CFDictionarySetValue(q,kSecAttrAccount,a);CFRelease(a);
 CFDictionarySetValue(q,kSecUseDataProtectionKeychain,kCFBooleanTrue);
 CFDictionarySetValue(q,kSecAttrSynchronizable,kCFBooleanFalse);
 CFDictionarySetValue(q,kSecUseAuthenticationUI,kSecUseAuthenticationUIFail);
 return q;
}
static int nw_put(const char *account,const void *bytes,int length) {
 CFMutableDictionaryRef q=nw_query(account);
 CFDataRef data=CFDataCreate(NULL,bytes,length);
 CFMutableDictionaryRef attrs=CFDictionaryCreateMutable(NULL,0,&kCFTypeDictionaryKeyCallBacks,&kCFTypeDictionaryValueCallBacks);
 CFDictionarySetValue(attrs,kSecValueData,data);
 OSStatus result=SecItemUpdate(q,attrs);
 if(result==errSecItemNotFound){
  CFDictionarySetValue(q,kSecValueData,data);
  CFDictionarySetValue(q,kSecAttrAccessible,kSecAttrAccessibleWhenUnlockedThisDeviceOnly);
  result=SecItemAdd(q,NULL);
 }
 CFRelease(attrs);CFRelease(data);CFRelease(q);return result==errSecSuccess;
}
static int nw_read(const char *account,void *buffer,int capacity) {
 CFMutableDictionaryRef q=nw_query(account);CFTypeRef data=NULL;
 CFDictionarySetValue(q,kSecReturnData,kCFBooleanTrue);
 CFDictionarySetValue(q,kSecMatchLimit,kSecMatchLimitOne);
 OSStatus status=SecItemCopyMatching(q,&data);CFRelease(q);
 if(status!=errSecSuccess||!data)return -1;
 if(CFGetTypeID(data)!=CFDataGetTypeID()){CFRelease(data);return -1;}
 CFIndex size=CFDataGetLength((CFDataRef)data);
 if(size<1||size>capacity){CFRelease(data);return -1;}
 memcpy(buffer,CFDataGetBytePtr((CFDataRef)data),size);CFRelease(data);return (int)size;
}
static int nw_delete(const char *account){CFMutableDictionaryRef q=nw_query(account);OSStatus s=SecItemDelete(q);CFRelease(q);return s==errSecSuccess||s==errSecItemNotFound;}
*/
import "C"

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"github.com/waltwang/nestworth-go/internal/assistant"
	"regexp"
	"unsafe"
)

// Set only by an audited release build using -ldflags -X. No environment or
// settings override is accepted. Existing ad-hoc packaging leaves this empty:
// credentials and provider activation stay unavailable, including dev builds.
var releaseTeamID string
var teamPattern = regexp.MustCompile(`^[A-Z0-9]{10}$`)

type keychain struct{ account, requirement string }

func New(scope string) assistant.Credentials {
	if !teamPattern.MatchString(releaseTeamID) || scope == "" {
		return unavailable{}
	}
	sum := sha256.Sum256([]byte(scope))
	k := &keychain{account: hex.EncodeToString(sum[:]), requirement: `anchor apple generic and identifier "com.nestworth.app" and certificate leaf[subject.OU] = "` + releaseTeamID + `"`}
	if !k.Available() {
		return unavailable{}
	}
	return k
}
func (k *keychain) Available() bool {
	q := C.CString(k.requirement)
	defer C.free(unsafe.Pointer(q))
	return C.nw_trusted(q) == 1
}
func (k *keychain) Put(ctx context.Context, key []byte) error {
	if ctx.Err() != nil || !k.Available() || len(key) < 8 || len(key) > 4096 {
		return errUnavailable
	}
	a := C.CString(k.account)
	defer C.free(unsafe.Pointer(a))
	if C.nw_put(a, unsafe.Pointer(&key[0]), C.int(len(key))) != 1 {
		return errUnavailable
	}
	return nil
}
func (k *keychain) With(ctx context.Context, fn func([]byte) error) error {
	if ctx.Err() != nil || !k.Available() {
		return errUnavailable
	}
	a := C.CString(k.account)
	defer C.free(unsafe.Pointer(a))
	buffer := make([]byte, 4096)
	defer clear(buffer)
	n := int(C.nw_read(a, unsafe.Pointer(&buffer[0]), C.int(len(buffer))))
	if n < 8 || ctx.Err() != nil {
		return errUnavailable
	}
	return fn(buffer[:n])
}
func (k *keychain) Delete(ctx context.Context) error {
	if ctx.Err() != nil || !k.Available() {
		return errUnavailable
	}
	a := C.CString(k.account)
	defer C.free(unsafe.Pointer(a))
	if C.nw_delete(a) != 1 {
		return errUnavailable
	}
	return nil
}
