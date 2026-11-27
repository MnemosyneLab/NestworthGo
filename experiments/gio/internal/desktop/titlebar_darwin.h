#ifndef NESTWORTH_TITLEBAR_DARWIN_H
#define NESTWORTH_TITLEBAR_DARWIN_H
#include <stdint.h>

enum {
    NestworthTitlebarWaiting = 0,
    NestworthTitlebarReady = 1,
    NestworthTitlebarIntegrated = 2,
    NestworthTitlebarUnavailable = 3
};

typedef struct {
    int widthPx, heightPx, fullSize, fullscreen, transition, visibleButtons;
    double outerWidth, outerHeight;
} NestworthTitlebarProbe;

void nestworthTitlebarAttach(uintptr_t token, uintptr_t view);
void nestworthTitlebarDetach(uintptr_t token);
void nestworthTitlebarRefresh(uintptr_t token);
// SetMode and Probe run synchronously through Gio's public Window.Run.
void nestworthTitlebarSetMode(uintptr_t token, int enabled, int dark, double width, double height);
NestworthTitlebarProbe nestworthTitlebarProbe(uintptr_t token);
void nestworthTitlebarUpdate(uintptr_t token, int dark, double width, double height,
	double x1, double y1, double w1, double h1,
	double x2, double y2, double w2, double h2);
void nestworthTitlebarChanged(uintptr_t token, int status, double right, double bottom,
	double center, double top, double left, double safeRight);
#endif
