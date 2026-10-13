// trace.c - logging opengl32.dll proxy for Clusterball.exe (forwards to the system GL)
#include <windows.h>
#include <stdio.h>
#include <stdarg.h>

static HMODULE g_real;
static FILE *g_log;
static CRITICAL_SECTION g_cs;
static int g_frame;

static void L(const char *fmt, ...) {
    if (!g_log) return;
    va_list a; va_start(a, fmt);
    EnterCriticalSection(&g_cs);
    vfprintf(g_log, fmt, a); fputc(10, g_log); fflush(g_log);
    LeaveCriticalSection(&g_cs);
    va_end(a);
}

static void *R(const char *n) {
    if (!g_real) {
        char p[MAX_PATH]; GetSystemDirectoryA(p, MAX_PATH); strcat(p, "\\opengl32.dll");
        g_real = LoadLibraryA(p);
    }
    return (void *)GetProcAddress(g_real, n);
}

#define EXP __declspec(dllexport)
// Each wrapper: log, then forward. T = return type, N = name, P = param list, A = arg names, F = printf fmt, V = values
#define W(T, N, P, A, F, ...) \
    EXP T __stdcall N P __asm__("_" #N); \
    EXP T __stdcall N P { static T (__stdcall *real_) P; if (!real_) real_ = R(#N); L(#N " " F, ##__VA_ARGS__); return real_ A; }
#define WV(N, P, A, F, ...) \
    EXP void __stdcall N P __asm__("_" #N); \
    EXP void __stdcall N P { static void (__stdcall *real_) P; if (!real_) real_ = R(#N); L(#N " " F, ##__VA_ARGS__); real_ A; }

typedef unsigned int GLenum, GLuint, GLbitfield; typedef int GLint, GLsizei; typedef float GLfloat; typedef double GLdouble;
typedef unsigned char GLubyte, GLboolean; typedef void GLvoid;

WV(glAlphaFunc, (GLenum f, GLfloat r), (f, r), "%x %g", f, r)
WV(glArrayElement, (GLint i), (i), "%d", i)
WV(glBegin, (GLenum m), (m), "%x", m)
WV(glBindTexture, (GLenum t, GLuint n), (t, n), "%x %u", t, n)
WV(glBlendFunc, (GLenum s, GLenum d), (s, d), "%x %x", s, d)
WV(glCallList, (GLuint l), (l), "%u", l)
WV(glClearColor, (GLfloat r, GLfloat g, GLfloat b, GLfloat a), (r, g, b, a), "%g %g %g %g", r, g, b, a)
WV(glColor4f, (GLfloat r, GLfloat g, GLfloat b, GLfloat a), (r, g, b, a), "%g %g %g %g", r, g, b, a)
WV(glColor4ub, (GLubyte r, GLubyte g, GLubyte b, GLubyte a), (r, g, b, a), "%u %u %u %u", r, g, b, a)
WV(glColorPointer, (GLint s, GLenum t, GLsizei st, const GLvoid *p), (s, t, st, p), "size=%d type=%x stride=%d p=%p", s, t, st, p)
WV(glDeleteLists, (GLuint l, GLsizei r), (l, r), "%u %d", l, r)
WV(glDeleteTextures, (GLsizei n, const GLuint *t), (n, t), "%d", n)
WV(glDepthFunc, (GLenum f), (f), "%x", f)
WV(glDepthMask, (GLboolean f), (f), "%u", f)
WV(glDisable, (GLenum c), (c), "%x", c)
WV(glDisableClientState, (GLenum c), (c), "%x", c)
WV(glDrawBuffer, (GLenum m), (m), "%x", m)
WV(glDrawElements, (GLenum m, GLsizei n, GLenum t, const GLvoid *i), (m, n, t, i), "mode=%x n=%d type=%x idx=%p", m, n, t, i)
WV(glEnable, (GLenum c), (c), "%x", c)
WV(glEnableClientState, (GLenum c), (c), "%x", c)
WV(glEnd, (void), (), "")
WV(glEndList, (void), (), "")
WV(glFinish, (void), (), "")
WV(glFlush, (void), (), "")
WV(glFogf, (GLenum p, GLfloat v), (p, v), "%x %g", p, v)
EXP void __stdcall glFogfv(GLenum p, const GLfloat *v) __asm__("_glFogfv");
EXP void __stdcall glFogfv(GLenum p, const GLfloat *v) { static void (__stdcall *f)(GLenum, const GLfloat *); if (!f) f = R("glFogfv"); L("glFogfv %x %g %g %g %g", p, v[0], p == 0xB66 ? v[1] : 0, p == 0xB66 ? v[2] : 0, p == 0xB66 ? v[3] : 0); f(p, v); }
WV(glFogi, (GLenum p, GLint v), (p, v), "%x %x", p, v)
W(GLuint, glGenLists, (GLsizei r), (r), "%d", r)
W(GLenum, glGetError, (void), (), "")
EXP void __stdcall glGetIntegerv(GLenum p, GLint *d) __asm__("_glGetIntegerv");
EXP void __stdcall glGetIntegerv(GLenum p, GLint *d) {
    static void (__stdcall *real_)(GLenum, GLint *);
    if (!real_) real_ = R("glGetIntegerv");
    real_(p, d);
    // The 2004 exe sizes a probe texture from MAX_TEXTURE_SIZE; 32768 makes it allocate 4 GB. Cap it.
    if (p == 0xD33 && *d > 2048) *d = 2048;
    L("glGetIntegerv %x -> %d", p, *d);
}
EXP const GLubyte *__stdcall glGetString(GLenum n) __asm__("_glGetString");
EXP const GLubyte *__stdcall glGetString(GLenum n) { static const GLubyte *(__stdcall *f)(GLenum); if (!f) f = R("glGetString"); const GLubyte *s = f(n); L("glGetString %x -> %s", n, s ? (const char *)s : "(null)"); return s; }
WV(glGetTexImage, (GLenum t, GLint l, GLenum fm, GLenum ty, GLvoid *p), (t, l, fm, ty, p), "tgt=%x lvl=%d fmt=%x type=%x", t, l, fm, ty)
WV(glHint, (GLenum t, GLenum m), (t, m), "%x %x", t, m)
WV(glListBase, (GLuint b), (b), "%u", b)
WV(glLoadIdentity, (void), (), "")
WV(glMatrixMode, (GLenum m), (m), "%x", m)
WV(glNewList, (GLuint l, GLenum m), (l, m), "%u %x", l, m)
WV(glPixelStorei, (GLenum p, GLint v), (p, v), "%x %d", p, v)
WV(glPixelTransferf, (GLenum p, GLfloat v), (p, v), "%x %g", p, v)
WV(glPopMatrix, (void), (), "")
WV(glPushMatrix, (void), (), "")
WV(glRecti, (GLint a, GLint b, GLint c, GLint d), (a, b, c, d), "%d %d %d %d", a, b, c, d)
WV(glShadeModel, (GLenum m), (m), "%x", m)
WV(glTexCoord2f, (GLfloat s, GLfloat t), (s, t), "%g %g", s, t)
WV(glTexCoordPointer, (GLint s, GLenum t, GLsizei st, const GLvoid *p), (s, t, st, p), "size=%d type=%x stride=%d p=%p", s, t, st, p)
WV(glTexEnvi, (GLenum t, GLenum p, GLint v), (t, p, v), "%x %x %x", t, p, v)
WV(glTexImage2D, (GLenum t, GLint l, GLint ifm, GLsizei w, GLsizei h, GLint b, GLenum fm, GLenum ty, const GLvoid *p), (t, l, ifm, w, h, b, fm, ty, p), "tgt=%x lvl=%d ifmt=%x %dx%d fmt=%x type=%x", t, l, ifm, w, h, fm, ty)
WV(glTexParameteri, (GLenum t, GLenum p, GLint v), (t, p, v), "%x %x %x", t, p, v)
WV(glTexSubImage2D, (GLenum t, GLint l, GLint x, GLint y, GLsizei w, GLsizei h, GLenum fm, GLenum ty, const GLvoid *p), (t, l, x, y, w, h, fm, ty, p), "tgt=%x lvl=%d at %d,%d %dx%d fmt=%x type=%x", t, l, x, y, w, h, fm, ty)
WV(glTranslated, (GLdouble x, GLdouble y, GLdouble z), (x, y, z), "%g %g %g", x, y, z)
WV(glTranslatef, (GLfloat x, GLfloat y, GLfloat z), (x, y, z), "%g %g %g", x, y, z)
WV(glVertex2f, (GLfloat x, GLfloat y), (x, y), "%g %g", x, y)
WV(glVertex2i, (GLint x, GLint y), (x, y), "%d %d", x, y)
WV(glVertexPointer, (GLint s, GLenum t, GLsizei st, const GLvoid *p), (s, t, st, p), "size=%d type=%x stride=%d p=%p", s, t, st, p)

W(HGLRC, wglCreateContext, (HDC dc), (dc), "%p", dc)
W(BOOL, wglDeleteContext, (HGLRC rc), (rc), "%p", rc)
W(BOOL, wglMakeCurrent, (HDC dc, HGLRC rc), (dc, rc), "%p %p", dc, rc)
EXP PROC __stdcall wglGetProcAddress(LPCSTR n) __asm__("_wglGetProcAddress");
EXP PROC __stdcall wglGetProcAddress(LPCSTR n) { static PROC (__stdcall *f)(LPCSTR); if (!f) f = R("wglGetProcAddress"); PROC p = f(n); L("wglGetProcAddress %s -> %p", n, p); return p; }

BOOL WINAPI DllMain(HINSTANCE h, DWORD why, LPVOID r) {
    if (why == DLL_PROCESS_ATTACH) {
        InitializeCriticalSection(&g_cs);
        { extern void compat_init(void); compat_init(); }
        { extern void *g_pass[]; extern const char *g_pass_names[]; R("glBegin"); for (int i = 0; g_pass_names[i]; i++) g_pass[i] = (void *)GetProcAddress(g_real, g_pass_names[i]); }
        char v[8];
        if (GetEnvironmentVariableA("CB_GLTRACE", v, sizeof v) && v[0] == '1') {
            g_log = fopen("gltrace.log", "w");
            if (g_log) setvbuf(g_log, NULL, _IOFBF, 1 << 16);
        }
    } else if (why == DLL_PROCESS_DETACH && g_log) fclose(g_log);
    return TRUE;
}

void *gl_real(const char *n) { return R(n); }
void gl_log(const char *s) { L("%s", s); }
int backend_d3d = 0;
BOOL backend_swap(HDC dc, int sec) { return FALSE; }
void backend_set(const char *k, double v) { (void)k; (void)v; } // test-script "set": no settings in the tracer
