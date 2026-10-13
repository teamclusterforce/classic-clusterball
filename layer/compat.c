// compat.c - import-table patches that make the 2004 Clusterball.exe behave on current Windows
#include <windows.h>
#include <string.h>
#include <stdio.h>
#include <stdarg.h>
#include <stdlib.h>

// The exe passes an uninitialised 148-byte DEVMODE (dmSize garbage); modern Windows then fails the call
// and the exe hits an int3. Call the real function with a proper DEVMODEA and copy back what the exe can hold.
// In windowed mode the game resolution (e.g. 852x480) is no real display mode; it is appended after the real list,
// otherwise the exe's mode search at 0x524ca0 runs off the end and hits an int3.
int g_game_w, g_game_h; // set when the game resolution is patched
extern int g_windowed;
static BOOL WINAPI my_EnumDisplaySettingsA(LPCSTR dev, DWORD mode, DEVMODEA *dm) {
    static DWORD real_count = 0xFFFFFFFF;
    DEVMODEA t;
    memset(&t, 0, sizeof t);
    t.dmSize = sizeof t;
    BOOL ok = mode < real_count ? EnumDisplaySettingsA(dev, mode, &t) : FALSE;
    if (!ok && (int)mode >= 0 && g_windowed && g_game_w) {
        if (real_count == 0xFFFFFFFF) real_count = mode;
        if (mode == real_count) {
            EnumDisplaySettingsA(dev, ENUM_CURRENT_SETTINGS, &t);
            t.dmPelsWidth = g_game_w; t.dmPelsHeight = g_game_h; t.dmBitsPerPel = 32;
            ok = TRUE;
        }
    }
    if (ok) memcpy(dm, &t, 148);
    if (mode == 0 || !ok || mode == real_count) {
        extern void CLOG(const char *fmt, ...);
        CLOG("EnumDisplaySettings #%lu ok=%d %lux%lu bpp=%lu; game wants %ux%u", mode, ok, t.dmPelsWidth, t.dmPelsHeight,
             t.dmBitsPerPel, *(WORD *)0xb11e3a, *(WORD *)0xb11e3c);
    }
    return ok;
}

void CLOG(const char *fmt, ...) {
    FILE *f = fopen("compat.log", "a");
    if (!f) return;
    va_list a;
    va_start(a, fmt);
    vfprintf(f, fmt, a);
    va_end(a);
    fputc('\n', f);
    fclose(f);
}

// --- Windowed mode: the exe believes it switched the display; it actually renders into a framed window. ---
int g_windowed = 1;
int g_tw = 1920, g_th = 1080; // render size (Resolution; default: the monitor's native size)
int g_mode_w, g_mode_h; // display mode the exe asked for (0 = desktop)
HWND g_main;
int g_fullscreen = 1; // 1: borderless window covering the monitor, 0: framed window; Alt+Enter toggles

// Real client size of the game window (the exe's GetClientRect is hooked to report the game mode; this module's
// calls are not). The render size is stretched to it on Present.
static void client_size(int *w, int *h) {
    RECT r;
    if (g_main && GetClientRect(g_main, &r) && r.right > 0 && r.bottom > 0) { *w = r.right; *h = r.bottom; }
    else { *w = g_tw; *h = g_th; }
}
static void fit_window(HWND w, int cw, int ch);
static void apply_style(HWND w);

// Game space (what the exe thinks the screen is) -> 4:3 box inside the real client area.
void game_box(float *s, int *ox, int *oy) {
    int gw = g_mode_w ? g_mode_w : 640, gh = g_mode_h ? g_mode_h : 480;
    float sx = (float)g_tw / gw, sy = (float)g_th / gh;
    *s = sx < sy ? sx : sy;
    float ar = sx / sy;
    if (ar > 0.99f && ar < 1.01f) { *s = sx; *ox = 0; *oy = 0; return; } // matches the backend's full-target mapping
    *ox = (int)((g_tw - gw * *s) / 2);
    *oy = (int)((g_th - gh * *s) / 2);
}

static WNDPROC g_orig_proc;

// Mouse messages arrive in real client pixels; the menus expect game-space pixels.
static LRESULT CALLBACK sub_proc(HWND w, UINT m, WPARAM wp, LPARAM lp) {
    if (m >= WM_MOUSEFIRST && m <= WM_MOUSELAST && m != WM_MOUSEWHEEL) {
        float s; int ox, oy;
        game_box(&s, &ox, &oy);
        int cw, ch;
        client_size(&cw, &ch);
        float px = (short)LOWORD(lp) * (float)g_tw / cw, py = (short)HIWORD(lp) * (float)g_th / ch;
        int x = (int)((px - ox) / s), y = (int)((py - oy) / s);
        lp = MAKELPARAM((WORD)(short)x, (WORD)(short)y);
    }
    // The game moves its own cursor from DirectInput deltas; like in fullscreen, the system cursor is hidden and
    // kept inside the client area while the window is active, and released when it is not.
    if (m == WM_SETCURSOR && LOWORD(lp) == HTCLIENT) { SetCursor(NULL); return TRUE; }
    if (m == WM_ACTIVATE || m == WM_SETFOCUS || m == WM_KILLFOCUS || m == WM_SIZE || m == WM_MOVE) {
        int active = (m == WM_ACTIVATE) ? LOWORD(wp) != WA_INACTIVE : (m == WM_KILLFOCUS ? 0 : GetForegroundWindow() == w);
        if (active) {
            RECT r; POINT a = {0, 0}, b;
            GetClientRect(w, &r);
            b.x = r.right; b.y = r.bottom;
            ClientToScreen(w, &a); ClientToScreen(w, &b);
            RECT c = {a.x, a.y, b.x, b.y};
            ClipCursor(&c);
        } else {
            ClipCursor(NULL);
        }
    }
    // Alt+Enter switches between borderless fullscreen and a framed window (the render size stays).
    if (m == WM_SYSKEYDOWN && wp == VK_RETURN && (lp & (1 << 29))) {
        if (!(lp & (1 << 30))) { // not a key repeat
            g_fullscreen = !g_fullscreen;
            apply_style(w);
            fit_window(w, 0, 0);
            CLOG("Alt+Enter: %s", g_fullscreen ? "fullscreen" : "window");
        }
        return 0;
    }
    if (m == WM_SYSCHAR && wp == VK_RETURN) return 0; // no beep
    // A fullscreen game minimises itself when it loses focus; a window must not.
    if (m == WM_ACTIVATEAPP && !wp) return 0;
    if (m == WM_SYSCOMMAND && (wp & 0xFFF0) == SC_MINIMIZE && GetForegroundWindow() != w) return 0;
    return CallWindowProcA(g_orig_proc, w, m, wp, lp);
}

static BOOL WINAPI my_CloseWindow(HWND w) {
    if (w == g_main) { CLOG("CloseWindow (minimise) suppressed"); return TRUE; }
    return CloseWindow(w);
}

static BOOL WINAPI my_ShowWindow(HWND w, int cmd) {
    if (w == g_main && (cmd == SW_MINIMIZE || cmd == SW_SHOWMINIMIZED || cmd == SW_SHOWMINNOACTIVE || cmd == SW_FORCEMINIMIZE)) {
        CLOG("ShowWindow(%d) suppressed", cmd);
        return TRUE;
    }
    return ShowWindow(w, cmd);
}

#define FRAMED_STYLE (WS_OVERLAPPED | WS_CAPTION | WS_SYSMENU | WS_MINIMIZEBOX)
static void apply_style(HWND w) {
    DWORD st = (DWORD)GetWindowLongA(w, GWL_STYLE) & ~(WS_POPUP | FRAMED_STYLE | WS_MAXIMIZE | WS_THICKFRAME);
    SetWindowLongA(w, GWL_STYLE, (LONG)(st | (g_fullscreen ? WS_POPUP : FRAMED_STYLE)));
}

// Fullscreen: the window covers its monitor. Window: client area at the render size, or the largest size of the
// same aspect that fits the work area, centred. The exe's requested size (cw, ch) does not matter.
static void fit_window(HWND w, int cw, int ch) {
    MONITORINFO mi;
    mi.cbSize = sizeof mi;
    GetMonitorInfoA(MonitorFromWindow(w, MONITOR_DEFAULTTOPRIMARY), &mi);
    if (g_fullscreen) {
        RECT m = mi.rcMonitor;
        SetWindowPos(w, HWND_TOP, m.left, m.top, m.right - m.left, m.bottom - m.top, SWP_NOACTIVATE | SWP_FRAMECHANGED);
        return;
    }
    RECT wa = mi.rcWork, fr = {0, 0, 100, 100};
    DWORD st = (DWORD)GetWindowLongA(w, GWL_STYLE), ex = (DWORD)GetWindowLongA(w, GWL_EXSTYLE);
    AdjustWindowRectEx(&fr, st, FALSE, ex);
    int bw = fr.right - fr.left - 100, bh = fr.bottom - fr.top - 100;
    int maxw = wa.right - wa.left - bw, maxh = wa.bottom - wa.top - bh;
    double k = 1;
    if (g_tw * k > maxw) k = (double)maxw / g_tw;
    if (g_th * k > maxh) k = (double)maxh / g_th;
    cw = (int)(g_tw * k); ch = (int)(g_th * k);
    int x = wa.left + (wa.right - wa.left - cw - bw) / 2, y = wa.top + (wa.bottom - wa.top - ch - bh) / 2;
    SetWindowPos(w, HWND_NOTOPMOST, x, y, cw + bw, ch + bh, SWP_NOACTIVATE | SWP_FRAMECHANGED);
}

static LONG WINAPI my_ChangeDisplaySettingsA(DEVMODEA *dm, DWORD flags) {
    // On exit the game "restores" the desktop mode (explicitly or with NULL); in a window that must change nothing,
    // otherwise the last frames and the mouse mapping are computed for a 2560x1440 game screen.
    if (!dm || (g_mode_w && dm->dmPelsWidth == (DWORD)GetSystemMetrics(SM_CXSCREEN) && dm->dmPelsHeight == (DWORD)GetSystemMetrics(SM_CYSCREEN))) {
        CLOG("ChangeDisplaySettingsA restore (%lux%lu) ignored", dm ? dm->dmPelsWidth : 0, dm ? dm->dmPelsHeight : 0);
        return DISP_CHANGE_SUCCESSFUL;
    }
    if (dm->dmFields & DM_PELSWIDTH) { g_mode_w = dm->dmPelsWidth; g_mode_h = dm->dmPelsHeight; }
    CLOG("ChangeDisplaySettingsA %dx%d flags=%lx", g_mode_w, g_mode_h, flags);
    if (g_main && g_mode_w) fit_window(g_main, g_mode_w, g_mode_h);
    return DISP_CHANGE_SUCCESSFUL;
}

static HWND WINAPI my_CreateWindowExA(DWORD ex, LPCSTR cls, LPCSTR title, DWORD st, int x, int y, int w, int h,
                                      HWND parent, HMENU menu, HINSTANCE inst, LPVOID param) {
    int top = !parent && (st & WS_POPUP);
    if (top) {
        st = (st & ~(WS_POPUP | FRAMED_STYLE | WS_MAXIMIZE | WS_THICKFRAME)) | (g_fullscreen ? WS_POPUP : FRAMED_STYLE);
        ex &= ~WS_EX_TOPMOST;
    }
    if (top && title && !strcmp(title, "Clusterball")) title = "Clusterball Re-Enhanced"; // window and taskbar name
    HWND r = CreateWindowExA(ex, cls, title, st, x, y, w, h, parent, menu, inst, param);
    CLOG("CreateWindowExA '%s' st=%lx ex=%lx %d,%d %dx%d -> %p", title ? title : "", st, ex, x, y, w, h, (void *)r);
    if (top && r && !g_main) {
        g_main = r;
        // config.cfg has been read by now: horiz/vert_game_res drive the OpenGL mode switch at 0x4093ad.
        if (g_game_w) { *(WORD *)0xb11e3a = (WORD)g_game_w; *(WORD *)0xb11e3c = (WORD)g_game_h; }
        g_orig_proc = (WNDPROC)SetWindowLongA(r, GWL_WNDPROC, (LONG)sub_proc);
        fit_window(r, g_mode_w ? g_mode_w : 640, g_mode_h ? g_mode_h : 480);
    }
    return r;
}

static BOOL WINAPI my_MoveWindow(HWND w, int x, int y, int cw, int ch, BOOL rep) {
    CLOG("MoveWindow %p %d,%d %dx%d", (void *)w, x, y, cw, ch);
    if (w == g_main) { fit_window(w, cw, ch); return TRUE; }
    return MoveWindow(w, x, y, cw, ch, rep);
}

// The exe derives its bottom-up viewport origin from the client height; it must see the game mode size.
static BOOL WINAPI my_GetClientRect(HWND w, RECT *r) {
    if (w == g_main && g_mode_w && r) { r->left = r->top = 0; r->right = g_mode_w; r->bottom = g_mode_h; return TRUE; }
    return GetClientRect(w, r);
}

static int WINAPI my_GetSystemMetrics(int i) {
    if (g_mode_w && i == SM_CXSCREEN) return g_mode_w;
    if (g_mode_h && i == SM_CYSCREEN) return g_mode_h;
    return GetSystemMetrics(i);
}

// The exe re-centres the cursor in "screen" coordinates of its fullscreen mode.
static BOOL WINAPI my_SetCursorPos(int x, int y) {
    if (g_main) {
        float s; int ox, oy;
        game_box(&s, &ox, &oy);
        int cw, ch;
        client_size(&cw, &ch);
        POINT p = {(int)((ox + x * s) * cw / g_tw), (int)((oy + y * s) * ch / g_th)};
        ClientToScreen(g_main, &p);
        return SetCursorPos(p.x, p.y);
    }
    return SetCursorPos(x, y);
}

static int g_patched;
static void patch_iat(HMODULE exe, const char *dll, const char *func, void *repl) {
    BYTE *base = (BYTE *)exe;
    IMAGE_NT_HEADERS *nt = (IMAGE_NT_HEADERS *)(base + ((IMAGE_DOS_HEADER *)base)->e_lfanew);
    DWORD rva = nt->OptionalHeader.DataDirectory[IMAGE_DIRECTORY_ENTRY_IMPORT].VirtualAddress;
    if (!rva) return;
    for (IMAGE_IMPORT_DESCRIPTOR *d = (IMAGE_IMPORT_DESCRIPTOR *)(base + rva); d->Name; d++) {
        if (lstrcmpiA((char *)(base + d->Name), dll)) continue;
        IMAGE_THUNK_DATA *ot = (IMAGE_THUNK_DATA *)(base + (d->OriginalFirstThunk ? d->OriginalFirstThunk : d->FirstThunk));
        IMAGE_THUNK_DATA *ft = (IMAGE_THUNK_DATA *)(base + d->FirstThunk);
        for (; ot->u1.AddressOfData; ot++, ft++) {
            if (ot->u1.Ordinal & IMAGE_ORDINAL_FLAG) continue;
            IMAGE_IMPORT_BY_NAME *n = (IMAGE_IMPORT_BY_NAME *)(base + ot->u1.AddressOfData);
            if (lstrcmpA((char *)n->Name, func)) continue;
            DWORD old;
            VirtualProtect(&ft->u1.Function, sizeof(void *), PAGE_READWRITE, &old);
            ft->u1.Function = (DWORD)(ULONG_PTR)repl; g_patched++;
            VirtualProtect(&ft->u1.Function, sizeof(void *), old, &old);
        }
    }
}

// Glide is dead hardware; a system-wide glide2x.dll wrapper would otherwise win the renderer choice.
// A NULL handle takes the exe's own "no 3dfx card" path at 0x53e5b0.
static HMODULE WINAPI my_LoadLibraryA(LPCSTR name) {
    if (name) {
        const char *b = strrchr(name, '\\');
        b = b ? b + 1 : name;
        if (!lstrcmpiA(b, "glide2x.dll") || !lstrcmpiA(b, "glide3x.dll")) {
            SetLastError(ERROR_MOD_NOT_FOUND);
            return NULL;
        }
    }
    return LoadLibraryA(name);
}

// --- Frame statistics and framebuffer dumps (read back right before the swap, independent of what covers the window) ---
extern void *gl_real(const char *n);
extern int backend_d3d;                  // 1 = Direct3D 11 backend, 0 = forwarding GL proxy
extern BOOL backend_swap(HDC dc, int sec); // D3D backend: present (and dump when sec >= 0)
static DWORD g_t0, g_tsec;
static int g_frames, g_fps_frames;
static int g_shot_at[16], g_shots, g_shot_next;

static void dump_bmp(int sec) {
    void (__stdcall *rp)(int, int, int, int, unsigned, unsigned, void *) = gl_real("glReadPixels");
    void (__stdcall *rb)(unsigned) = gl_real("glReadBuffer");
    if (!rp || !g_main) return;
    RECT c;
    GetClientRect(g_main, &c);
    int w = c.right, h = c.bottom;
    if (w <= 0 || h <= 0) return;
    int stride = (w * 3 + 3) & ~3;
    BYTE *px = malloc((size_t)stride * h);
    if (!px) return;
    if (rb) rb(0x0405); // GL_BACK
    rp(0, 0, w, h, 0x80E0 /*GL_BGR*/, 0x1401, px); // rows bottom-up, as BMP wants
    char name[64];
    CreateDirectoryA("shots", NULL);
    sprintf(name, "shots\\frame_%02d.bmp", sec);
    FILE *f = fopen(name, "wb");
    if (f) {
        BITMAPFILEHEADER fh = {0x4D42, (DWORD)(sizeof(BITMAPFILEHEADER) + sizeof(BITMAPINFOHEADER) + stride * h), 0, 0,
                               sizeof(BITMAPFILEHEADER) + sizeof(BITMAPINFOHEADER)};
        BITMAPINFOHEADER ih = {sizeof ih, w, h, 1, 24, BI_RGB, 0, 0, 0, 0, 0};
        fwrite(&fh, sizeof fh, 1, f);
        fwrite(&ih, sizeof ih, 1, f);
        fwrite(px, (size_t)stride, h, f);
        fclose(f);
    }
    free(px);
    CLOG("dumped %s (%dx%d)", name, w, h);
}

// Test script (CB_SCRIPT=file): lines "<ms since first frame> <cmd> [args]" with the commands
//   move <dx> <dy> | click | keydown <DIK> | keyup <DIK> | shot <name> | quit
typedef struct { DWORD ms; char cmd[12]; char arg[48]; } ScriptLine;
static ScriptLine *g_script;
static int g_script_n, g_script_pos;
char g_shot_name[64];
extern void inj_mouse(int click, long dx, long dy);

extern BYTE g_inj_keys[256];

double cfg_num(const char *env, const char *key, double def);
static void script_load(void) {
    char path[MAX_PATH], line[128];
    if (!GetEnvironmentVariableA("CB_SCRIPT", path, sizeof path)) return;
    FILE *f = fopen(path, "r");
    if (!f) return;
    while (fgets(line, sizeof line, f)) {
        ScriptLine s;
        memset(&s, 0, sizeof s);
        if (sscanf(line, "%lu %11s %47[^\r\n]", &s.ms, s.cmd, s.arg) < 2) continue;
        g_script = realloc(g_script, (g_script_n + 1) * sizeof s);
        g_script[g_script_n++] = s;
    }
    fclose(f);
    { extern int g_script_mode; g_script_mode = cfg_num("CB_SCRIPTLOCK", "ScriptLock", 1) != 0; }
    CLOG("script %s: %d lines", path, g_script_n);
}

static int script_tick(DWORD t) { // returns 1 when a named shot is due this frame
    int shot = 0;
    while (g_script_pos < g_script_n && g_script[g_script_pos].ms <= t) {
        ScriptLine *s = &g_script[g_script_pos++];
        long a = 0, b = 0;
        sscanf(s->arg, "%ld %ld", &a, &b);
        if (!strcmp(s->cmd, "move")) inj_mouse(0, a, b);
        else if (!strcmp(s->cmd, "click")) inj_mouse(1, 0, 0);
        else if (!strcmp(s->cmd, "keydown")) g_inj_keys[a & 255] = 1;
        else if (!strcmp(s->cmd, "keyup")) g_inj_keys[a & 255] = 0;
        else if (!strcmp(s->cmd, "shot")) { snprintf(g_shot_name, sizeof g_shot_name, "%s", s->arg); shot = 1; }
        else if (!strcmp(s->cmd, "quit")) ExitProcess(0);
        else if (!strcmp(s->cmd, "set")) { // set <name> <value>: change a backend setting at run time
            char key[32]; double v = 0;
            if (sscanf(s->arg, "%31s %lf", key, &v) == 2) { extern void backend_set(const char *, double); backend_set(key, v); }
        }
        CLOG("script %lu ms: %s %s", t, s->cmd, s->arg);
        if (shot) break;
    }
    return shot;
}

static BOOL WINAPI my_SwapBuffers(HDC dc) {
    DWORD now = GetTickCount();
    if (!g_t0) g_t0 = g_tsec = now;
    g_frames++; g_fps_frames++;
    if (now - g_tsec >= 1000) {
        CLOG("t=%lus frames=%d fps=%d", (now - g_t0) / 1000, g_frames, g_fps_frames);
        { extern void input_stats_tick(void); input_stats_tick(); }
        g_fps_frames = 0; g_tsec = now;
    }
    int sec = -1;
    if (g_shot_next < g_shots && (int)((now - g_t0) / 1000) >= g_shot_at[g_shot_next]) sec = g_shot_at[g_shot_next++];
    if (script_tick(now - g_t0)) sec = 0;
    if (backend_d3d) return backend_swap(dc, sec);
    if (sec >= 0) dump_bmp(sec);
    return SwapBuffers(dc);
}

// Writes a 24-bit BMP; rows are given top-down.
void write_bmp24_topdown(const char *name, int w, int h, const BYTE *bgra, int pitch) {
    int stride = (w * 3 + 3) & ~3;
    BYTE *row = calloc(1, stride);
    FILE *f = fopen(name, "wb");
    if (f && row) {
        BITMAPFILEHEADER fh = {0x4D42, (DWORD)(sizeof(BITMAPFILEHEADER) + sizeof(BITMAPINFOHEADER) + stride * h), 0, 0,
                               sizeof(BITMAPFILEHEADER) + sizeof(BITMAPINFOHEADER)};
        BITMAPINFOHEADER ih = {sizeof ih, w, h, 1, 24, BI_RGB, 0, 0, 0, 0, 0};
        fwrite(&fh, sizeof fh, 1, f);
        fwrite(&ih, sizeof ih, 1, f);
        for (int y = h - 1; y >= 0; y--) {
            const BYTE *s = bgra + (size_t)y * pitch;
            for (int x = 0; x < w; x++) { row[x * 3] = s[x * 4]; row[x * 3 + 1] = s[x * 4 + 1]; row[x * 3 + 2] = s[x * 4 + 2]; }
            fwrite(row, stride, 1, f);
        }
    }
    if (f) fclose(f);
    free(row);
    CLOG("dumped %s (%dx%d)", name, w, h);
}

// The OpenGL renderer offers exactly 640x480 (gm_OGL_GetResolutions 0x51c0e0 and the mode request at 0x409164).
// Rewrite those push immediates so the game lays out and projects for a 16:9 screen; verified bytes only.
static int patch_push(DWORD va, DWORD expect, DWORD value) {
    BYTE *p = (BYTE *)(ULONG_PTR)va;
    if (p[0] != 0x68 || *(DWORD *)(p + 1) != expect) return 0;
    DWORD old;
    VirtualProtect(p + 1, 4, PAGE_EXECUTE_READWRITE, &old);
    *(DWORD *)(p + 1) = value;
    VirtualProtect(p + 1, 4, old, &old);
    FlushInstructionCache(GetCurrentProcess(), p, 5);
    return 1;
}

static void patch_game_res(int w, int h) {
    static const DWORD ws[] = {0x409180, 0x51c1c8, 0x51c1e3}, hs[] = {0x40917b, 0x51c1c1, 0x51c1de};
    int n = 0;
    for (int i = 0; i < 3; i++) n += patch_push(ws[i], 640, w) + patch_push(hs[i], 480, h);
    CLOG("game resolution %dx%d (%d of 6 pushes patched)", w, h, n);
    g_game_w = w; g_game_h = h;
}

// The XDream re-release (v1.4.0) put a "mod by xdgn" splash into Data.mrg (pictures/3). It is blitted at 0x49130a
// while loading, and a Sleep(1000) at 0x490d1f keeps it up afterwards. Both are patched in memory, bytes verified.
static int patch_bytes(DWORD va, const BYTE *expect, const BYTE *repl, int n) {
    BYTE *p = (BYTE *)(ULONG_PTR)va;
    if (memcmp(p, expect, n)) return 0;
    DWORD old;
    VirtualProtect(p, n, PAGE_EXECUTE_READWRITE, &old);
    memcpy(p, repl, n);
    VirtualProtect(p, n, old, &old);
    FlushInstructionCache(GetCurrentProcess(), p, n);
    return 1;
}
static void remove_splash(void) {
    static const BYTE call_blit[5] = {0xE8, 0x21, 0xC4, 0x09, 0x00}, nops[5] = {0x90, 0x90, 0x90, 0x90, 0x90};
    int a = patch_bytes(0x49130a, call_blit, nops, 5);
    int b = patch_push(0x490d1f, 1000, 0);
    CLOG("splash removed: blit %s, sleep %s", a ? "patched" : "NOT FOUND", b ? "patched" : "NOT FOUND");
}

// config.cfg is parsed after the window exists; the last call before the OpenGL mode switch at 0x4093ad that reads
// horiz/vert_game_res is GetDeviceCaps (0x4092c9). Setting the values there makes GameResolution independent of config.cfg.
static void force_game_res(void) {
    if (g_windowed && g_game_w) { *(WORD *)0xb11e3a = (WORD)g_game_w; *(WORD *)0xb11e3c = (WORD)g_game_h; }
}
static int WINAPI my_GetDeviceCaps(HDC dc, int i) {
    force_game_res();
    return GetDeviceCaps(dc, i);
}

// With the D3D backend there is no GL pixel format: the window only needs to accept the DXGI swap chain.
static int WINAPI my_ChoosePixelFormat(HDC dc, const PIXELFORMATDESCRIPTOR *p) { return 1; }
static BOOL WINAPI my_SetPixelFormat(HDC dc, int fmt, const PIXELFORMATDESCRIPTOR *p) { return TRUE; }

// --- Settings: environment variable CB_<KEY> wins over clusterball_d3d.ini [d3d] <Key>, which wins over the default.
int cfg_str(const char *env, const char *key, char *out, int n) {
    if (GetEnvironmentVariableA(env, out, n)) return 1;
    char ini[MAX_PATH], *s;
    GetModuleFileNameA(NULL, ini, MAX_PATH);
    if ((s = strrchr(ini, '\\'))) strcpy(s + 1, "clusterball_d3d.ini");
    GetPrivateProfileStringA("d3d", key, "", out, n, ini);
    return out[0] != 0;
}
double cfg_num(const char *env, const char *key, double def) {
    char v[32];
    return cfg_str(env, key, v, sizeof v) ? atof(v) : def;
}

// --- DirectInput mouse: optional scaling of the raw deltas (modern mice report many more counts per cm than in 2004)
// and a per-second log of what the game receives.
static double g_mouse_scale = 1.0, g_acc_x, g_acc_y;
static void *g_mouse_dev;
static long g_sum_x, g_sum_y, g_calls_state, g_calls_data;
static HRESULT(WINAPI *real_GetDeviceState)(void *, DWORD, void *);
static HRESULT(WINAPI *real_GetDeviceData)(void *, DWORD, void *, DWORD *, DWORD);
static HRESULT(WINAPI *real_SetCooperativeLevel)(void *, HWND, DWORD);
static HRESULT(WINAPI *real_SetProperty)(void *, const GUID *, const void *);
static HRESULT(WINAPI *real_CreateDevice)(void *, const GUID *, void **, void *);
static const GUID k_sysmouse = {0x6F1D2B60, 0xD5A0, 0x11CF, {0xBF, 0xC7, 0x44, 0x45, 0x53, 0x54, 0x00, 0x00}};

static long scale_delta(long d, double *acc) {
    if (g_mouse_scale == 1.0) return d;
    *acc += d * g_mouse_scale;
    long out = (long)*acc;
    *acc -= out;
    return out;
}

// --- Test driver (only with CB_SCRIPT): raw mouse counts, button and key presses injected into the game's own polling.
// Mouse actions run strictly one after another: a move is fed in steps of at most 300 counts per poll and must be
// used up before the next move or click starts. (One shared counter let a slow corner move swallow the next move.)
typedef struct { int click; long dx, dy; int polls; } MouseAct;
static MouseAct g_mq[64];
static int g_mq_head, g_mq_tail;
void inj_mouse(int click, long dx, long dy) {
    int next = (g_mq_tail + 1) % 64;
    if (next == g_mq_head) return;
    MouseAct a = {click, dx, dy, click ? 3 : 0};
    g_mq[g_mq_tail] = a;
    g_mq_tail = next;
}
BYTE g_inj_keys[256];     // DIK scan codes held down
static DWORD g_esc_until, g_esc_last; // compat_press_escape: Esc held until this tick
static void *g_kbd_dev;
int g_script_mode;

static long step(long *p) {
    long v = *p > 300 ? 300 : *p < -300 ? -300 : *p;
    *p -= v;
    return v;
}

// Next injected mouse delta and button state for one poll.
static void inj_poll(LONG *dx, LONG *dy, int *button) {
    *dx = *dy = 0; *button = 0;
    if (g_mq_head == g_mq_tail) return;
    MouseAct *a = &g_mq[g_mq_head];
    if (a->click) {
        *button = a->polls > 0;
        if (a->polls-- <= 0) g_mq_head = (g_mq_head + 1) % 64; // one poll released before the next action
        return;
    }
    *dx = step(&a->dx);
    *dy = step(&a->dy);
    if (!a->dx && !a->dy) g_mq_head = (g_mq_head + 1) % 64;
}

static HRESULT WINAPI my_GetDeviceState(void *self, DWORD cb, void *data) {
    HRESULT hr = real_GetDeviceState(self, cb, data);
    // In script mode the game must not see the real mouse/keyboard: DirectInput runs in background mode and would
    // pass on whatever the user does in other windows.
    if (g_script_mode && SUCCEEDED(hr) && (self == g_mouse_dev || self == g_kbd_dev)) memset(data, 0, cb);
    if (self == g_mouse_dev && SUCCEEDED(hr) && cb >= 12) {
        LONG *m = data;
        LONG idx, idy;
        int ib;
        inj_poll(&idx, &idy, &ib);
        m[0] = scale_delta(m[0], &g_acc_x) + idx;
        m[1] = scale_delta(m[1], &g_acc_y) + idy;
        if (ib && cb >= 16) ((BYTE *)data)[12] = 0x80;
        g_sum_x += labs(m[0]); g_sum_y += labs(m[1]); g_calls_state++;
    }
    if (self == g_kbd_dev && SUCCEEDED(hr) && cb == 256) {
        BYTE *k = data;
        for (int i = 0; i < 256; i++) if (g_inj_keys[i]) k[i] = 0x80;
        if ((LONG)(GetTickCount() - g_esc_until) < 0) k[0x01] = 0x80; // DIK_ESCAPE, see compat_press_escape
    }
    return hr;
}

// On exit the game shows the "community / www.clusterball.de - mod by xdgn" picture and waits for Esc. The renderer
// recognises that picture and calls this instead of drawing it: Esc is reported as held for a moment.
void compat_press_escape(void) {
    DWORD now = GetTickCount();
    if (now - g_esc_last < 1000) return; // once per second while the picture is still being drawn
    g_esc_last = now;
    g_esc_until = now + 200;
    CLOG("exit picture: Esc pressed");
}

static HRESULT WINAPI my_GetDeviceData(void *self, DWORD cb, void *rgdod, DWORD *n, DWORD flags) {
    HRESULT hr = real_GetDeviceData(self, cb, rgdod, n, flags);
    if (self == g_mouse_dev && SUCCEEDED(hr) && rgdod && n) {
        for (DWORD i = 0; i < *n; i++) {
            DWORD *od = (DWORD *)((BYTE *)rgdod + i * cb); // dwOfs, dwData, ...
            if (od[0] == 0) { od[1] = (DWORD)scale_delta((LONG)od[1], &g_acc_x); g_sum_x += labs((LONG)od[1]); }
            else if (od[0] == 4) { od[1] = (DWORD)scale_delta((LONG)od[1], &g_acc_y); g_sum_y += labs((LONG)od[1]); }
        }
        g_calls_data++;
    }
    return hr;
}

static HRESULT WINAPI my_SetCooperativeLevel(void *self, HWND w, DWORD flags) {
    if (self == g_mouse_dev) CLOG("dinput mouse SetCooperativeLevel flags=%lx", flags);
    return real_SetCooperativeLevel(self, w, flags);
}

static HRESULT WINAPI my_SetProperty(void *self, const GUID *prop, const void *ph) {
    if (self == g_mouse_dev && (ULONG_PTR)prop < 0x10000) {
        const DWORD *p = ph; // DIPROPHEADER (4 DWORDs) + dwData
        CLOG("dinput mouse SetProperty %lu value=%lu", (unsigned long)(ULONG_PTR)prop, p[4]);
    }
    return real_SetProperty(self, prop, ph);
}

static void patch_vtbl(void **vt, int idx, void *fn, void **orig) {
    if (!*orig) *orig = vt[idx];
    DWORD old;
    VirtualProtect(&vt[idx], sizeof(void *), PAGE_READWRITE, &old);
    vt[idx] = fn;
    VirtualProtect(&vt[idx], sizeof(void *), old, &old);
}

static HRESULT WINAPI my_CreateDevice(void *self, const GUID *g, void **dev, void *outer) {
    HRESULT hr = real_CreateDevice(self, g, dev, outer);
    if (SUCCEEDED(hr) && g && !memcmp(g, &k_sysmouse, sizeof *g)) {
        g_mouse_dev = *dev;
        void **vt = *(void ***)*dev;
        patch_vtbl(vt, 6, my_SetProperty, (void **)&real_SetProperty);
        patch_vtbl(vt, 9, my_GetDeviceState, (void **)&real_GetDeviceState);
        patch_vtbl(vt, 10, my_GetDeviceData, (void **)&real_GetDeviceData);
        patch_vtbl(vt, 13, my_SetCooperativeLevel, (void **)&real_SetCooperativeLevel);
        CLOG("dinput mouse device %p, scale %.3f", *dev, g_mouse_scale);
    }
    static const GUID k_syskbd = {0x6F1D2B61, 0xD5A0, 0x11CF, {0xBF, 0xC7, 0x44, 0x45, 0x53, 0x54, 0x00, 0x00}};
    if (SUCCEEDED(hr) && g && !memcmp(g, &k_syskbd, sizeof *g)) {
        g_kbd_dev = *dev;
        patch_vtbl(*(void ***)*dev, 9, my_GetDeviceState, (void **)&real_GetDeviceState);
        CLOG("dinput keyboard device %p", *dev);
    }
    return hr;
}

static HRESULT(WINAPI *real_DirectInputCreateA)(HINSTANCE, DWORD, void **, void *);
static HRESULT WINAPI my_DirectInputCreateA(HINSTANCE h, DWORD ver, void **di, void *outer) {
    HRESULT hr = real_DirectInputCreateA(h, ver, di, outer);
    CLOG("DirectInputCreateA version %lx -> %lx", ver, hr);
    if (SUCCEEDED(hr) && di && *di) patch_vtbl(*(void ***)*di, 3, my_CreateDevice, (void **)&real_CreateDevice);
    return hr;
}

void input_stats_tick(void) {
    if (g_mouse_dev && (g_calls_state || g_calls_data))
        CLOG("mouse: |dx|=%ld |dy|=%ld per s, GetDeviceState %ld/s, GetDeviceData %ld/s", g_sum_x, g_sum_y, g_calls_state, g_calls_data);
    g_sum_x = g_sum_y = g_calls_state = g_calls_data = 0;
}

void compat_init(void) {
    HMODULE exe = GetModuleHandleA(NULL);
    { FILE *f = fopen("compat.log", "w"); if (f) fclose(f); }
    script_load();
    patch_iat(exe, "GDI32.dll", "SwapBuffers", (void *)my_SwapBuffers);
    patch_iat(exe, "GDI32.dll", "GetDeviceCaps", (void *)my_GetDeviceCaps);
    {
        HMODULE di = LoadLibraryA("dinput.dll");
        real_DirectInputCreateA = di ? (void *)GetProcAddress(di, "DirectInputCreateA") : NULL;
        if (real_DirectInputCreateA) patch_iat(exe, "DINPUT.dll", "DirectInputCreateA", (void *)my_DirectInputCreateA);
        // DirectInput delivers raw counts and ignores the Windows pointer speed; apply it explicitly (speed 10 = 1.0,
        // the same table Windows uses without "enhance pointer precision"), times a base factor for this game.
        g_mouse_scale = cfg_num("CB_MOUSE", "MouseScale", 0.35);
        if (cfg_num("CB_MOUSEWIN", "MouseFollowWindows", 1) != 0) {
            static const double k_speed[21] = {0, 1 / 32., 1 / 16., 1 / 8., 2 / 8., 3 / 8., 4 / 8., 5 / 8., 6 / 8., 7 / 8., 1.0,
                                               1.25, 1.5, 1.75, 2.0, 2.25, 2.5, 2.75, 3.0, 3.25, 3.5};
            int sp = 10;
            SystemParametersInfoA(SPI_GETMOUSESPEED, 0, &sp, 0);
            if (sp >= 1 && sp <= 20) g_mouse_scale *= k_speed[sp];
            CLOG("windows pointer speed %d -> mouse scale %.3f", sp, g_mouse_scale);
        }
    }
    if (backend_d3d) {
        patch_iat(exe, "GDI32.dll", "ChoosePixelFormat", (void *)my_ChoosePixelFormat);
        patch_iat(exe, "GDI32.dll", "SetPixelFormat", (void *)my_SetPixelFormat);
    }
    char shots[96];
    if (GetEnvironmentVariableA("CB_SHOTS", shots, sizeof shots)) { // e.g. "3,6,10,15"
        for (char *p = strtok(shots, ","); p && g_shots < 16; p = strtok(NULL, ",")) g_shot_at[g_shots++] = atoi(p);
    }
    patch_iat(exe, "USER32.dll", "EnumDisplaySettingsA", (void *)my_EnumDisplaySettingsA);
    patch_iat(exe, "KERNEL32.dll", "LoadLibraryA", (void *)my_LoadLibraryA);
    // Fullscreen: 1 (default) borderless at the monitor, 0 framed window, 2 the game's own display mode switch.
    int fs = (int)cfg_num("CB_FULLSCREEN", "Fullscreen", 1);
    if (fs == 2) g_windowed = 0;
    g_fullscreen = fs != 0;
    if (g_windowed) {
        patch_iat(exe, "USER32.dll", "ChangeDisplaySettingsA", (void *)my_ChangeDisplaySettingsA);
        patch_iat(exe, "USER32.dll", "CreateWindowExA", (void *)my_CreateWindowExA);
        patch_iat(exe, "USER32.dll", "MoveWindow", (void *)my_MoveWindow);
        patch_iat(exe, "USER32.dll", "GetSystemMetrics", (void *)my_GetSystemMetrics);
        patch_iat(exe, "USER32.dll", "SetCursorPos", (void *)my_SetCursorPos);
        patch_iat(exe, "USER32.dll", "CloseWindow", (void *)my_CloseWindow);
        patch_iat(exe, "USER32.dll", "GetClientRect", (void *)my_GetClientRect);
        patch_iat(exe, "USER32.dll", "ShowWindow", (void *)my_ShowWindow);
    }
    char res[32];
    // Render size: Resolution=WxH, or (empty / native) the primary monitor's size.
    g_tw = GetSystemMetrics(SM_CXSCREEN); g_th = GetSystemMetrics(SM_CYSCREEN);
    if (cfg_str("CB_RES", "Resolution", res, sizeof res)) {
        int w, h;
        if (sscanf(res, "%dx%d", &w, &h) == 2 && w >= 320 && h >= 240) { g_tw = w; g_th = h; }
    }
    CLOG("exe=%p imports patched=%d", (void *)exe, g_patched);
    if (cfg_num("CB_NOSPLASH", "RemoveSplash", 1) != 0) remove_splash();
    if (g_windowed) {
        // Default: 480 lines at the window's aspect (852x480 for 16:9). GameResolution=640x480 restores the original.
        int gw = (int)(480.0 * g_tw / g_th + 0.5) & ~1, gh = 480;
        if (cfg_str("CB_GAMERES", "GameResolution", res, sizeof res)) sscanf(res, "%dx%d", &gw, &gh);
        // Always set it, also for 640x480: config.cfg may still hold another size from an earlier run.
        patch_game_res(gw, gh);
    }
}

int compat_patched_count(void) { return g_patched; }
