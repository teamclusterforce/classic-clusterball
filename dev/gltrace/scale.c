// scale.c - render the exe's fixed 640x480 OpenGL mode at any client size
// 3D (glFrustum) fills the whole client area with a widened frustum; 2D (glOrtho) stays 4:3 and centred.
#include <windows.h>
#include <stdio.h>

typedef unsigned int GLenum, GLbitfield;
typedef int GLint, GLsizei;
typedef double GLdouble;

extern void *gl_real(const char *n);
extern void gl_log(const char *s);
extern void game_box(float *s, int *ox, int *oy);
extern int g_tw, g_th, g_mode_w, g_mode_h, g_windowed;

#define EXP __declspec(dllexport)
#define REAL(T, N, P) static T(__stdcall * real_##N) P; if (!real_##N) real_##N = gl_real(#N)

static int g_3d;                  // current projection is a perspective frustum
static int vx, vy, vw = 640, vh = 480; // viewport in game space
static int sx_, sy_, sw = 640, sh = 480; // scissor in game space

static int gw(void) { return g_mode_w ? g_mode_w : 640; }
static int gh(void) { return g_mode_h ? g_mode_h : 480; }

// Only a full-screen 3D viewport is stretched to the full client width.
static int wide(void) { return g_windowed && g_3d && vx == 0 && vy == 0 && vw == gw() && vh == gh(); }

static void map_rect(int x, int y, int w, int h, int *o) {
    if (!g_windowed) { o[0] = x; o[1] = y; o[2] = w; o[3] = h; return; }
    if (wide()) {
        float kx = (float)g_tw / gw(), ky = (float)g_th / gh();
        o[0] = (int)(x * kx); o[1] = (int)(y * ky); o[2] = (int)(w * kx + 0.5f); o[3] = (int)(h * ky + 0.5f);
        return;
    }
    float s; int ox, oy;
    game_box(&s, &ox, &oy);
    o[0] = ox + (int)(x * s); o[1] = oy + (int)(y * s); o[2] = (int)(w * s + 0.5f); o[3] = (int)(h * s + 0.5f);
}

static void apply(void) {
    REAL(void, glViewport, (GLint, GLint, GLsizei, GLsizei));
    REAL(void, glScissor, (GLint, GLint, GLsizei, GLsizei));
    int r[4];
    map_rect(vx, vy, vw, vh, r);
    real_glViewport(r[0], r[1], r[2], r[3]);
    map_rect(sx_, sy_, sw, sh, r);
    real_glScissor(r[0], r[1], r[2], r[3]);
}

EXP void __stdcall glViewport(GLint x, GLint y, GLsizei w, GLsizei h) __asm__("_glViewport");
EXP void __stdcall glViewport(GLint x, GLint y, GLsizei w, GLsizei h) {
    vx = x; vy = y; vw = w; vh = h;
    char b[96]; sprintf(b, "glViewport %d %d %d %d", x, y, w, h); gl_log(b);
    apply();
}

EXP void __stdcall glScissor(GLint x, GLint y, GLsizei w, GLsizei h) __asm__("_glScissor");
EXP void __stdcall glScissor(GLint x, GLint y, GLsizei w, GLsizei h) {
    sx_ = x; sy_ = y; sw = w; sh = h;
    char b[96]; sprintf(b, "glScissor %d %d %d %d", x, y, w, h); gl_log(b);
    apply();
}

EXP void __stdcall glFrustum(GLdouble l, GLdouble r, GLdouble b, GLdouble t, GLdouble n, GLdouble f) __asm__("_glFrustum");
EXP void __stdcall glFrustum(GLdouble l, GLdouble r, GLdouble b, GLdouble t, GLdouble n, GLdouble f) {
    REAL(void, glFrustum, (GLdouble, GLdouble, GLdouble, GLdouble, GLdouble, GLdouble));
    g_3d = 1;
    if (wide()) {
        // Keep the vertical field of view, widen the horizontal one to the client aspect.
        double k = ((double)g_tw / g_th) / ((double)gw() / gh());
        l *= k; r *= k;
    }
    char s[160]; sprintf(s, "glFrustum %g %g %g %g %g %g", l, r, b, t, n, f); gl_log(s);
    real_glFrustum(l, r, b, t, n, f);
    apply();
}

EXP void __stdcall glOrtho(GLdouble l, GLdouble r, GLdouble b, GLdouble t, GLdouble n, GLdouble f) __asm__("_glOrtho");
EXP void __stdcall glOrtho(GLdouble l, GLdouble r, GLdouble b, GLdouble t, GLdouble n, GLdouble f) {
    REAL(void, glOrtho, (GLdouble, GLdouble, GLdouble, GLdouble, GLdouble, GLdouble));
    g_3d = 0;
    char s[160]; sprintf(s, "glOrtho %g %g %g %g %g %g", l, r, b, t, n, f); gl_log(s);
    real_glOrtho(l, r, b, t, n, f);
    apply();
}

// The pillarbox bars are never drawn by the game: clear the whole client area along with its own clear.
EXP void __stdcall glClear(GLbitfield m) __asm__("_glClear");
EXP void __stdcall glClear(GLbitfield m) {
    REAL(void, glClear, (GLbitfield));
    REAL(void, glScissor, (GLint, GLint, GLsizei, GLsizei));
    char s[48]; sprintf(s, "glClear %x", m); gl_log(s);
    if (g_windowed) {
        real_glScissor(0, 0, g_tw, g_th);
        real_glClear(m);
        apply();
        return;
    }
    real_glClear(m);
}
