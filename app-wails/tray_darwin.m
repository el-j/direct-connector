// tray_darwin.m — macOS menu-bar status-item tray.
// Uses dispatch_async(main_queue) so it is safe to call from any goroutine.
// NO NSApp delegate is replaced; Wails' delegate is left intact.

#import <Cocoa/Cocoa.h>

// Callback exported by tray_darwin.go
extern void onTrayAction(int action);

static NSStatusItem   *statusItem  = NULL;
static id              trayTarget  = NULL;

// ── Menu-action target ────────────────────────────────────────────────────────

@interface TrayTarget : NSObject
- (void)menuItemClicked:(NSMenuItem *)sender;
@end

@implementation TrayTarget
- (void)menuItemClicked:(NSMenuItem *)sender {
    onTrayAction((int)sender.tag);
}
@end

// ── C entry points ────────────────────────────────────────────────────────────

// setupTrayIcon creates the NSStatusItem and menu on the main thread.
// iconData / iconLen may be NULL/0; the button title "DC" is used as fallback.
// NSData copies iconData immediately, so the caller may free it after return.
void setupTrayIcon(const char *iconData, int iconLen) {
    // Copy icon bytes into NSData NOW (synchronously), before dispatch_async,
    // so the caller can safely free the source buffer after this function returns.
    NSData *iconNSData = nil;
    if (iconData != NULL && iconLen > 0) {
        iconNSData = [NSData dataWithBytes:iconData length:(NSUInteger)iconLen];
    }

    dispatch_async(dispatch_get_main_queue(), ^{
        trayTarget = [[TrayTarget alloc] init];

        statusItem = [[NSStatusBar systemStatusBar]
                      statusItemWithLength:NSSquareStatusItemLength];

        if (iconNSData != nil) {
            NSImage *img = [[NSImage alloc] initWithData:iconNSData];
            [img setSize:NSMakeSize(18, 18)];
            // template = YES lets macOS invert the icon for dark/light menu bar
            img.template = YES;
            statusItem.button.image = img;
        } else {
            statusItem.button.title = @"DC";
        }
        statusItem.button.toolTip = @"Direct Connector";

        NSMenu *menu = [[NSMenu alloc] init];
        [menu setAutoenablesItems:NO];

        NSMenuItem *openItem = [[NSMenuItem alloc]
            initWithTitle:@"Open Direct Connector"
                   action:@selector(menuItemClicked:)
            keyEquivalent:@""];
        openItem.target = trayTarget;
        openItem.tag    = 0;
        [menu addItem:openItem];

        [menu addItem:[NSMenuItem separatorItem]];

        NSMenuItem *helpItem = [[NSMenuItem alloc]
            initWithTitle:@"Help"
                   action:@selector(menuItemClicked:)
            keyEquivalent:@""];
        helpItem.target = trayTarget;
        helpItem.tag    = 1;
        [menu addItem:helpItem];

        [menu addItem:[NSMenuItem separatorItem]];

        NSMenuItem *exitItem = [[NSMenuItem alloc]
            initWithTitle:@"Exit"
                   action:@selector(menuItemClicked:)
            keyEquivalent:@""];
        exitItem.target = trayTarget;
        exitItem.tag    = 2;
        [menu addItem:exitItem];

        statusItem.menu = menu;
    });
}

void teardownTrayIcon(void) {
    dispatch_async(dispatch_get_main_queue(), ^{
        if (statusItem != NULL) {
            [[NSStatusBar systemStatusBar] removeStatusItem:statusItem];
            statusItem   = NULL;
            trayTarget   = NULL;
        }
    });
}
