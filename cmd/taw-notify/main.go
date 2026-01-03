//go:build darwin

// taw-notify is a helper binary for macOS notifications with action buttons.
// It runs as an app bundle to receive proper notification permissions.
//
// Usage:
//
//	taw-notify --title "Title" --body "Body" [--icon /path/to/icon.png] [--action "Accept"] [--action "Decline"]
//
// Output:
//
//	Prints one of: ACTION_0, ACTION_1, ..., CLICKED, DISMISSED, TIMEOUT
package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Foundation -framework UserNotifications -framework AppKit

#import <Foundation/Foundation.h>
#import <UserNotifications/UserNotifications.h>
#import <AppKit/AppKit.h>

// Result storage
static char resultBuffer[64] = "";
static BOOL resultReady = NO;

void setResult(const char* result) {
    strncpy(resultBuffer, result, sizeof(resultBuffer) - 1);
    resultBuffer[sizeof(resultBuffer) - 1] = '\0';
    resultReady = YES;
}

const char* getResult() {
    return resultBuffer;
}

BOOL isResultReady() {
    return resultReady;
}

// Notification delegate to handle actions
@interface NotificationDelegate : NSObject <UNUserNotificationCenterDelegate>
@end

@implementation NotificationDelegate

- (void)userNotificationCenter:(UNUserNotificationCenter *)center
       willPresentNotification:(UNNotification *)notification
         withCompletionHandler:(void (^)(UNNotificationPresentationOptions))completionHandler {
    completionHandler(UNNotificationPresentationOptionBanner | UNNotificationPresentationOptionSound);
}

- (void)userNotificationCenter:(UNUserNotificationCenter *)center
didReceiveNotificationResponse:(UNNotificationResponse *)response
         withCompletionHandler:(void (^)(void))completionHandler {
    NSString *actionId = response.actionIdentifier;

    if ([actionId hasPrefix:@"ACTION_"]) {
        setResult([actionId UTF8String]);
    } else if ([actionId isEqualToString:UNNotificationDefaultActionIdentifier]) {
        setResult("CLICKED");
    } else if ([actionId isEqualToString:UNNotificationDismissActionIdentifier]) {
        setResult("DISMISSED");
    } else {
        setResult("UNKNOWN");
    }

    completionHandler();

    // Stop the run loop after a short delay to ensure completion handler runs
    dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(0.1 * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
        [NSApp stop:nil];
        // Post a dummy event to ensure the run loop exits
        NSEvent *event = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
                                            location:NSMakePoint(0, 0)
                                       modifierFlags:0
                                           timestamp:0
                                        windowNumber:0
                                             context:nil
                                             subtype:0
                                               data1:0
                                               data2:0];
        [NSApp postEvent:event atStart:YES];
    });
}

@end

static NotificationDelegate *notifDelegate = nil;

void setupNotificationCenter(int actionCount, const char** actionTitles) {
    UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];

    notifDelegate = [[NotificationDelegate alloc] init];
    center.delegate = notifDelegate;

    // Request authorization synchronously (wait for completion)
    dispatch_semaphore_t sem = dispatch_semaphore_create(0);
    __block BOOL granted = NO;

    [center requestAuthorizationWithOptions:(UNAuthorizationOptionAlert | UNAuthorizationOptionSound)
                          completionHandler:^(BOOL g, NSError * _Nullable error) {
        granted = g;
        dispatch_semaphore_signal(sem);
    }];

    dispatch_semaphore_wait(sem, dispatch_time(DISPATCH_TIME_NOW, 5 * NSEC_PER_SEC));

    if (!granted) {
        return;
    }

    // Create actions if provided
    if (actionCount > 0) {
        NSMutableArray *actions = [NSMutableArray arrayWithCapacity:actionCount];
        for (int i = 0; i < actionCount && i < 5; i++) {
            NSString *actionId = [NSString stringWithFormat:@"ACTION_%d", i];
            NSString *title = [NSString stringWithUTF8String:actionTitles[i]];
            UNNotificationAction *action = [UNNotificationAction actionWithIdentifier:actionId
                                                                                 title:title
                                                                               options:UNNotificationActionOptionForeground];
            [actions addObject:action];
        }

        UNNotificationCategory *category = [UNNotificationCategory categoryWithIdentifier:@"TAW_PROMPT"
                                                                                   actions:actions
                                                                         intentIdentifiers:@[]
                                                                                   options:UNNotificationCategoryOptionCustomDismissAction];

        [center setNotificationCategories:[NSSet setWithObject:category]];
    }
}

void sendNotification(const char* title, const char* body, const char* iconPath, int hasActions) {
    UNUserNotificationCenter *center = [UNUserNotificationCenter currentNotificationCenter];

    UNMutableNotificationContent *content = [[UNMutableNotificationContent alloc] init];
    content.title = [NSString stringWithUTF8String:title];
    content.body = [NSString stringWithUTF8String:body];
    content.sound = [UNNotificationSound defaultSound];

    if (hasActions) {
        content.categoryIdentifier = @"TAW_PROMPT";
    }

    // Add icon as attachment if provided
    if (iconPath && strlen(iconPath) > 0) {
        NSString *path = [NSString stringWithUTF8String:iconPath];
        NSURL *iconURL = [NSURL fileURLWithPath:path];

        if ([[NSFileManager defaultManager] fileExistsAtPath:path]) {
            NSError *error = nil;
            UNNotificationAttachment *attachment = [UNNotificationAttachment attachmentWithIdentifier:@"icon"
                                                                                                  URL:iconURL
                                                                                              options:nil
                                                                                                error:&error];
            if (attachment && !error) {
                content.attachments = @[attachment];
            }
        }
    }

    // Trigger immediately
    UNTimeIntervalNotificationTrigger *trigger = [UNTimeIntervalNotificationTrigger triggerWithTimeInterval:0.1 repeats:NO];

    NSString *reqId = [[NSUUID UUID] UUIDString];
    UNNotificationRequest *request = [UNNotificationRequest requestWithIdentifier:reqId
                                                                           content:content
                                                                           trigger:trigger];

    [center addNotificationRequest:request withCompletionHandler:^(NSError * _Nullable error) {
        if (error) {
            NSLog(@"Error sending notification: %@", error);
        }
    }];
}

void runApp(int timeoutSeconds) {
    @autoreleasepool {
        [NSApplication sharedApplication];
        // Use accessory mode so we don't show in dock
        [NSApp setActivationPolicy:NSApplicationActivationPolicyAccessory];
        [NSApp finishLaunching];

        // Set up timeout
        dispatch_after(dispatch_time(DISPATCH_TIME_NOW, (int64_t)(timeoutSeconds * NSEC_PER_SEC)), dispatch_get_main_queue(), ^{
            if (!isResultReady()) {
                setResult("TIMEOUT");
                [NSApp stop:nil];
                NSEvent *event = [NSEvent otherEventWithType:NSEventTypeApplicationDefined
                                                    location:NSMakePoint(0, 0)
                                               modifierFlags:0
                                                   timestamp:0
                                                windowNumber:0
                                                     context:nil
                                                     subtype:0
                                                       data1:0
                                                       data2:0];
                [NSApp postEvent:event atStart:YES];
            }
        });

        [NSApp run];
    }
}
*/
import "C"

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"unsafe"
)

type stringSlice []string

func (s *stringSlice) String() string {
	return strings.Join(*s, ", ")
}

func (s *stringSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func main() {
	var (
		title   = flag.String("title", "", "Notification title")
		body    = flag.String("body", "", "Notification body")
		icon    = flag.String("icon", "", "Path to icon image")
		timeout = flag.Int("timeout", 30, "Timeout in seconds")
		actions stringSlice
	)
	flag.Var(&actions, "action", "Action button title (can be repeated, max 5)")
	flag.Parse()

	if *title == "" {
		fmt.Fprintln(os.Stderr, "Error: --title is required")
		os.Exit(1)
	}

	// Setup notification center with actions
	var cActions []*C.char
	for _, a := range actions {
		cActions = append(cActions, C.CString(a))
	}
	defer func() {
		for _, ca := range cActions {
			C.free(unsafe.Pointer(ca))
		}
	}()

	var actionPtr **C.char
	if len(cActions) > 0 {
		actionPtr = &cActions[0]
	}

	C.setupNotificationCenter(C.int(len(actions)), actionPtr)

	// Send notification
	cTitle := C.CString(*title)
	cBody := C.CString(*body)
	cIcon := C.CString(*icon)
	defer C.free(unsafe.Pointer(cTitle))
	defer C.free(unsafe.Pointer(cBody))
	defer C.free(unsafe.Pointer(cIcon))

	hasActions := C.int(0)
	if len(actions) > 0 {
		hasActions = C.int(1)
	}
	C.sendNotification(cTitle, cBody, cIcon, hasActions)

	// Run app loop with timeout
	C.runApp(C.int(*timeout))

	// Print result
	result := C.GoString(C.getResult())
	if result == "" {
		result = "TIMEOUT"
	}
	fmt.Println(result)
}
