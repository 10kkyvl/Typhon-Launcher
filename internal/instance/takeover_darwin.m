#import <Foundation/Foundation.h>
#include <stdlib.h>
#include "takeover_darwin.h"

extern void typhonInstanceMessage(char *message);

static id typhonObserver = nil;

char *typhonTempDir(void) {
	return strdup([NSTemporaryDirectory() UTF8String]);
}

// Second launches arrive as a distributed notification named after the
// single-instance id, the launch data being its object: the format Wails
// sends and reads on macOS.
void typhonObserve(const char *name) {
	NSString *n = [NSString stringWithUTF8String:name];
	typhonObserver = [[NSDistributedNotificationCenter defaultCenter]
		addObserverForName:n
		object:nil
		queue:nil
		usingBlock:^(NSNotification *note) {
			if (note.object != nil) {
				NSString *m = (NSString *)note.object;
				typhonInstanceMessage((char *)m.UTF8String);
			}
		}];
}

void typhonStopObserving(void) {
	if (typhonObserver != nil) {
		[[NSDistributedNotificationCenter defaultCenter] removeObserver:typhonObserver];
		typhonObserver = nil;
	}
}
