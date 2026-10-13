// gld3d.c - the OpenGL 1.1 subset used by Clusterball.exe (2004), implemented on Direct3D 11
// Built as opengl32.dll next to the exe. Window, display-mode and input fixes live in compat.c.
#define COBJMACROS
#include <windows.h>
#include <d3d11.h>
#include <dxgi.h>
#include <math.h>
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdarg.h>

typedef unsigned int GLenum, GLuint, GLbitfield;
typedef int GLint, GLsizei;
typedef float GLfloat, GLclampf;
typedef double GLdouble;
typedef unsigned char GLubyte, GLboolean;
typedef void GLvoid;

#define GLF(ret, name, params)                                                        \
    __declspec(dllexport) ret __stdcall name params __asm__("_" #name);               \
    __declspec(dllexport) ret __stdcall name params

// ---------------------------------------------------------------- shared with compat.c
extern int g_tw, g_th, g_mode_w, g_mode_h, g_windowed;
extern HWND g_main;
extern void compat_init(void);
extern void write_bmp24_topdown(const char *name, int w, int h, const BYTE *bgra, int pitch);
int backend_d3d = 1;
void *gl_real(const char *n) { return NULL; }

static FILE *g_dlog;
static void DLOG(const char *fmt, ...) {
    if (!g_dlog) g_dlog = fopen("d3d.log", "w");
    if (!g_dlog) return;
    va_list a;
    va_start(a, fmt);
    vfprintf(g_dlog, fmt, a);
    va_end(a);
    fputc('\n', g_dlog);
    fflush(g_dlog);
}

// ---------------------------------------------------------------- matrices (column-major, as GL)
typedef struct { float m[16]; } M4;
static void m_ident(M4 *a) { memset(a, 0, sizeof *a); a->m[0] = a->m[5] = a->m[10] = a->m[15] = 1; }
static void m_mul(M4 *r, const M4 *a, const M4 *b) {
    M4 t;
    for (int c = 0; c < 4; c++)
        for (int w = 0; w < 4; w++)
            t.m[c * 4 + w] = a->m[0 * 4 + w] * b->m[c * 4 + 0] + a->m[1 * 4 + w] * b->m[c * 4 + 1] +
                             a->m[2 * 4 + w] * b->m[c * 4 + 2] + a->m[3 * 4 + w] * b->m[c * 4 + 3];
    *r = t;
}

static M4 mv_st[32], pj_st[8], tx_st[2][8];
static int mv_top, pj_top, tx_top[2];
static GLenum g_matmode = 0x1700;

// ---------------------------------------------------------------- state
typedef struct { GLint size; GLenum type; GLsizei stride; const BYTE *ptr; int on; } Arr;
static struct {
    float color[4], tc[2][2];
    int tex_on[2], env[2], unit, cunit;
    GLuint bound[2];
    int blend_on; GLenum bsrc, bdst;
    int alpha_on; GLenum afunc; float aref;
    int depth_on; GLenum dfunc; int dmask;
    int scissor_on;
    int fog_on; GLenum fogmode; float fogstart, fogend, fogdens, fogcolor[4];
    int vx, vy, vw, vh, sx, sy, sw, sh;
    float clearcolor[4];
    Arr va, ca, ta[2];
    GLint unpack[6], pack[6];
    float tscale[4], tbias[4];
    GLuint listbase;
} S;

static int game_w(void) { return g_mode_w ? g_mode_w : 640; }
static int game_h(void) { return g_mode_h ? g_mode_h : 480; }

static M4 *cur_mat(void) {
    if (g_matmode == 0x1701) return &pj_st[pj_top];
    if (g_matmode == 0x1702) return &tx_st[S.unit][tx_top[S.unit]];
    return &mv_st[mv_top];
}

static void state_defaults(void) {
    memset(&S, 0, sizeof S);
    for (int i = 0; i < 4; i++) S.color[i] = 1, S.tscale[i] = 1;
    S.env[0] = S.env[1] = 0x2100;
    S.bsrc = 1; S.bdst = 0;
    S.afunc = 0x207; S.dfunc = 0x201; S.dmask = 1;
    S.fogmode = 0x800; S.fogend = 1; S.fogdens = 1;
    S.vw = S.sw = 640; S.vh = S.sh = 480;
    S.unpack[5] = S.pack[5] = 4;
    m_ident(&mv_st[0]); m_ident(&pj_st[0]); m_ident(&tx_st[0][0]); m_ident(&tx_st[1][0]);
}

// ---------------------------------------------------------------- textures
#define MAXL 13
typedef struct Tex {
    GLuint name;
    int w[MAXL], h[MAXL];
    BYTE *lv[MAXL];
    GLenum minf, magf, ws, wt;
    ID3D11Texture2D *t;
    ID3D11ShaderResourceView *srv;
    int nlev, dirty;
    int replaced, dynamic; // dynamic: updated with glTexSubImage2D, never replaced
    unsigned gen, font_gen;  // pixel uploads so far / gen at the last font-atlas lookup
    struct HdTex *font_hd;   // high-resolution replacement of the menu font atlas, if any
    int font_tall;           // its cells are twice as high: glyph quads are drawn at double height
    int ui_listed;           // TextureDump: hash written to textures\ui_textures.txt (drawn in 2D)
    int unrep_logged;        // LogUnreplaced: already reported
    struct Tex *next;
} Tex;
static Tex *tex_tab[1024];

static Tex *tex_get(GLuint n, int create) {
    Tex **b = &tex_tab[n & 1023];
    for (Tex *t = *b; t; t = t->next)
        if (t->name == n) return t;
    if (!create) return NULL;
    Tex *t = calloc(1, sizeof *t);
    t->name = n;
    t->minf = 0x2702; t->magf = 0x2601; t->ws = t->wt = 0x2901;
    t->next = *b;
    *b = t;
    return t;
}

static void tex_release_gpu(Tex *t) {
    if (t->srv) ID3D11ShaderResourceView_Release(t->srv);
    if (t->t) ID3D11Texture2D_Release(t->t);
    t->srv = NULL; t->t = NULL; t->nlev = 0;
}

static void tex_free(GLuint n) {
    Tex **b = &tex_tab[n & 1023];
    for (Tex **p = b; *p; p = &(*p)->next)
        if ((*p)->name == n) {
            Tex *t = *p;
            *p = t->next;
            tex_release_gpu(t);
            for (int i = 0; i < MAXL; i++) free(t->lv[i]);
            free(t);
            return;
        }
}

// Component layout of a client pixel format.
static int fmt_comps(GLenum f) {
    switch (f) {
    case 0x1908: case 0x80E1: return 4; // RGBA, BGRA
    case 0x1907: case 0x80E0: return 3; // RGB, BGR
    case 0x190A: return 2;              // LUMINANCE_ALPHA
    default: return 1;                  // LUMINANCE, ALPHA, RED, ...
    }
}
static int type_size(GLenum t) {
    switch (t) {
    case 0x1402: case 0x1403: return 2; // SHORT, UNSIGNED_SHORT
    case 0x1404: case 0x1405: case 0x1406: return 4;
    default: return 1;
    }
}
static int packed16(GLenum t) { return t == 0x8033 || t == 0x8034 || t == 0x8363; }

static float read_comp(const BYTE *p, GLenum type) {
    switch (type) {
    case 0x1403: return *(const unsigned short *)p / 65535.f;
    case 0x1402: return *(const short *)p / 32767.f;
    case 0x1405: return (float)(*(const unsigned int *)p / 4294967295.0);
    case 0x1406: return *(const float *)p;
    case 0x1400: return *(const signed char *)p / 127.f;
    default: return *p / 255.f;
    }
}

// Client pixels (with GL_UNPACK_* rules and pixel transfer) -> RGBA8 rows of w pixels.
static void unpack_rect(const BYTE *src, int w, int h, GLenum fmt, GLenum type, BYTE *dst, int dpitch) {
    int comps = fmt_comps(fmt), ts = type_size(type);
    int psize = packed16(type) ? 2 : comps * ts;
    int rowlen = S.unpack[2] > 0 ? S.unpack[2] : w;
    int align = S.unpack[5] > 0 ? S.unpack[5] : 4;
    int spitch = (rowlen * psize + align - 1) / align * align;
    src += (size_t)S.unpack[3] * spitch + (size_t)S.unpack[4] * psize;
    int xfer = S.tscale[0] != 1 || S.tscale[1] != 1 || S.tscale[2] != 1 || S.tscale[3] != 1 ||
               S.tbias[0] != 0 || S.tbias[1] != 0 || S.tbias[2] != 0 || S.tbias[3] != 0;
    for (int y = 0; y < h; y++) {
        const BYTE *s = src + (size_t)y * spitch;
        BYTE *d = dst + (size_t)y * dpitch;
        for (int x = 0; x < w; x++, s += psize, d += 4) {
            float c[4] = {0, 0, 0, 1}, v[4];
            if (packed16(type)) {
                unsigned p = *(const unsigned short *)s;
                if (type == 0x8033) { c[0] = (p >> 12) / 15.f; c[1] = ((p >> 8) & 15) / 15.f; c[2] = ((p >> 4) & 15) / 15.f; c[3] = (p & 15) / 15.f; }
                else if (type == 0x8034) { c[0] = (p >> 11) / 31.f; c[1] = ((p >> 6) & 31) / 31.f; c[2] = ((p >> 1) & 31) / 31.f; c[3] = (float)(p & 1); }
                else { c[0] = (p >> 11) / 31.f; c[1] = ((p >> 5) & 63) / 63.f; c[2] = (p & 31) / 31.f; }
            } else {
                for (int i = 0; i < comps; i++) v[i] = read_comp(s + i * ts, type);
                switch (fmt) {
                case 0x1908: c[0] = v[0]; c[1] = v[1]; c[2] = v[2]; c[3] = v[3]; break;
                case 0x80E1: c[0] = v[2]; c[1] = v[1]; c[2] = v[0]; c[3] = v[3]; break;
                case 0x1907: c[0] = v[0]; c[1] = v[1]; c[2] = v[2]; break;
                case 0x80E0: c[0] = v[2]; c[1] = v[1]; c[2] = v[0]; break;
                case 0x1909: c[0] = c[1] = c[2] = v[0]; break;
                case 0x190A: c[0] = c[1] = c[2] = v[0]; c[3] = v[1]; break;
                case 0x1906: c[0] = c[1] = c[2] = 1; c[3] = v[0]; break;
                default: c[0] = v[0]; break;
                }
            }
            for (int i = 0; i < 4; i++) {
                float f = xfer ? c[i] * S.tscale[i] + S.tbias[i] : c[i];
                f = f < 0 ? 0 : f > 1 ? 1 : f;
                d[i] = (BYTE)(f * 255.f + 0.5f);
            }
        }
    }
}

// ---------------------------------------------------------------- D3D11 objects
static ID3D11Device *dev;
static ID3D11DeviceContext *ctx;
static IDXGISwapChain *sc;
static ID3D11Texture2D *rt_tex, *ds_tex;
static ID3D11RenderTargetView *rtv;
static ID3D11DepthStencilView *dsv;
static int rt_w, rt_h, g_sync = 1, g_vsync_cfg = 1, g_latency = 1;
static int out_w, out_h, g_msaa = 4, g_ss = 1;      // client size, MSAA samples, supersample factor
static ID3D11Texture2D *res_tex;                     // MSAA resolve target when supersampling
static ID3D11ShaderResourceView *src_srv;            // input of the downscale pass
// Motion measurement: does the camera change every rendered frame, or only at the game's simulation rate?
static int g_frame_has3d, g_3d_frames, g_3d_changes, g_v_changes, g_ao_done, g_frame_cleared;
static int g_hd_loaded, g_hd_missing; // texture pack statistics (defined with the pack loader below)
static unsigned g_frame_vhash, g_last_vhash;
static M4 g_frame_mvp, g_last_mvp;
static DWORD g_motion_t;
static void motion_tick(void) {
    if (g_frame_has3d) {
        g_3d_frames++;
        if (memcmp(&g_frame_mvp, &g_last_mvp, sizeof g_last_mvp)) g_3d_changes++;
        if (g_frame_vhash != g_last_vhash) g_v_changes++;
        g_last_mvp = g_frame_mvp;
        g_last_vhash = g_frame_vhash;
    }
    g_frame_has3d = 0;
    g_ao_done = 0;
    g_frame_cleared = 0;
    DWORD now = GetTickCount();
    if (!g_motion_t) g_motion_t = now;
    if (now - g_motion_t >= 1000) {
        if (g_3d_frames) DLOG("motion: %d 3D frames/s, matrix changed in %d, first-draw vertices changed in %d; hd textures %d loaded, %d without pack file",
                              g_3d_frames, g_3d_changes, g_v_changes, g_hd_loaded, g_hd_missing);
        g_3d_frames = g_3d_changes = g_v_changes = 0;
        g_motion_t = now;
    }
}
static ID3D11VertexShader *down_vs;
static ID3D11PixelShader *down_ps;

// ---------------------------------------------------------------- post-processing
// Present: [downscale] -> post_tex -> CAS sharpening + colour grading -> back buffer.
// In-frame: ambient occlusion from the depth buffer, applied when the first 2D draw follows a fully redrawn 3D scene.
static ID3D11Texture2D *post_tex, *ao_tex;
static ID3D11RenderTargetView *post_rtv, *ao_rtv;
static ID3D11ShaderResourceView *post_srv, *ao_srv, *depth_srv;
static ID3D11PixelShader *post_ps, *ao_ps, *aoblur_ps;
static ID3D11Buffer *pbuf;
static float g_sharpen = 0.5f, g_contrast = 1.05f, g_saturation = 1.1f, g_gamma = 1.0f;
static int g_ao_on = 1;
static float g_ao_strength = 0.6f, g_ao_radius = 200.f; // radius in world units (100 = 1 m)

static M4 g_ao_proj;

static const char *k_post_hlsl =
    "cbuffer P : register(b1) { float4 grade; float4 aop; float4 aop2; };\n"
    "Texture2D src : register(t0);\n"
    "#if MS > 1\n"
    "Texture2DMS<float> dep : register(t1);\n"
    "float depthAt(int2 p) { return dep.Load(p, 0); }\n"
    "#else\n"
    "Texture2D<float> dep : register(t1);\n"
    "float depthAt(int2 p) { return dep.Load(int3(p, 0)); }\n"
    "#endif\n"
    "Texture2D<float> aot : register(t2);\n"
    "float4 vsq(uint id : SV_VertexID) : SV_Position {\n"
    "  float2 p = float2((id << 1) & 2, id & 2); return float4(p * float2(2, -2) + float2(-1, 1), 0, 1); }\n"
    "float4 psdown(float4 pos : SV_Position) : SV_Target {\n"
    "  int2 b = int2(pos.xy) * SS; float4 c = 0;\n"
    "  [unroll] for (int y = 0; y < SS; y++) [unroll] for (int x = 0; x < SS; x++) c += src.Load(int3(b + int2(x, y), 0));\n"
    "  return c / (SS * SS); }\n"
    "float3 px(int2 p) { return src.Load(int3(p, 0)).rgb; }\n"
    // Contrast-adaptive sharpening (after AMD FidelityFX CAS): weight from the local min/max of a 3x3 neighbourhood.
    "float4 pspost(float4 pos : SV_Position) : SV_Target {\n"
    "  int2 p = int2(pos.xy); float3 e = px(p);\n"
    "  if (grade.w > 0) {\n"
    "    float3 a = px(p + int2(-1, -1)), b = px(p + int2(0, -1)), c = px(p + int2(1, -1)), d = px(p + int2(-1, 0));\n"
    "    float3 f = px(p + int2(1, 0)), g = px(p + int2(-1, 1)), h = px(p + int2(0, 1)), i = px(p + int2(1, 1));\n"
    "    float3 mn = min(min(min(d, e), min(f, b)), h); mn += min(mn, min(min(a, c), min(g, i)));\n"
    "    float3 mx = max(max(max(d, e), max(f, b)), h); mx += max(mx, max(max(a, c), max(g, i)));\n"
    "    float3 amp = sqrt(saturate(min(mn, 2.0 - mx) / max(mx, 1e-4)));\n"
    "    float3 w = amp * (-1.0 / lerp(8.0, 5.0, grade.w));\n"
    "    e = saturate((b * w + d * w + f * w + h * w + e) / (1.0 + 4.0 * w)); }\n"
    "  e = saturate((e - 0.5) * grade.x + 0.5);\n"
    "  float l = dot(e, float3(0.299, 0.587, 0.114));\n"
    "  e = saturate(lerp(l.xxx, e, grade.y));\n"
    "  e = pow(e, 1.0 / grade.z);\n"
    "  return float4(e, 1); }\n"
    // aop = near, far, focal length in pixels, radius; aop2 = strength, target width, height
    "float lin(float d) { float z = d * 2 - 1; return 2 * aop.x * aop.y / ((aop.y + aop.x) - z * (aop.y - aop.x)); }\n"
    "float psao(float4 pos : SV_Position) : SV_Target {\n"
    "  int2 p = int2(pos.xy); int2 lim = int2(aop2.y - 1, aop2.z - 1);\n"
    "  float d = depthAt(p); if (d >= 0.99999) return 1;\n"
    "  float z = lin(d);\n"
    // local plane from the neighbours, so flat ground seen at a grazing angle does not occlude itself
    "  float zx = lin(depthAt(min(p + int2(1, 0), lim))) - z, zy = lin(depthAt(min(p + int2(0, 1), lim))) - z;\n"
    "  if (abs(zx) > aop.w || abs(zy) > aop.w) { zx = 0; zy = 0; }\n"
    "  float rpx = clamp(aop.z * aop.w / z, 2, 80);\n"
    "  float ang = frac(52.9829189 * frac(dot(pos.xy, float2(0.06711056, 0.00583715)))) * 6.2831853;\n"
    "  float occ = 0;\n"
    "  [unroll] for (int k = 0; k < 12; k++) {\n"
    "    float t = (k + 0.5) / 12.0, a = ang + k * 2.3999632;\n"
    "    float2 o = float2(cos(a), sin(a)) * rpx * t;\n"
    "    int2 q = clamp(p + int2(o), int2(0, 0), lim);\n"
    "    float expect = z + o.x * zx + o.y * zy, diff = expect - lin(depthAt(q));\n"
    "    occ += (diff > aop.w * 0.05 && diff < aop.w * 3) ? saturate(diff / aop.w) : 0; }\n"
    "  return 1 - aop2.x * occ / 12; }\n"
    "float4 psaoblur(float4 pos : SV_Position) : SV_Target {\n"
    "  int2 p = int2(pos.xy); float s = 0;\n"
    "  [unroll] for (int y = -2; y < 2; y++) [unroll] for (int x = -2; x < 2; x++) s += aot.Load(int3(p + int2(x, y), 0));\n"
    "  s /= 16; return float4(s, s, s, 1); }\n";

static void write_pbuf(void) {
    float pc[12] = {g_contrast, g_saturation, g_gamma, g_sharpen};
    M4 *p = &g_ao_proj;
    float m10 = p->m[10], m14 = p->m[14];
    pc[4] = m14 / (m10 - 1);                 // near
    pc[5] = m14 / (m10 + 1);                 // far
    pc[6] = p->m[5] * rt_h * 0.5f;           // focal length in pixels
    pc[7] = g_ao_radius;
    pc[8] = g_ao_strength; pc[9] = (float)rt_w; pc[10] = (float)rt_h;
    D3D11_MAPPED_SUBRESOURCE m;
    if (SUCCEEDED(ID3D11DeviceContext_Map(ctx, (ID3D11Resource *)pbuf, 0, D3D11_MAP_WRITE_DISCARD, 0, &m))) {
        memcpy(m.pData, pc, sizeof pc);
        ID3D11DeviceContext_Unmap(ctx, (ID3D11Resource *)pbuf, 0);
    }
}

static ID3D11RasterizerState *get_raster(int scissor);
static ID3D11BlendState *get_blend(int on, GLenum src, GLenum dst, int write_color);
static ID3D11DepthStencilState *get_depth(int on, GLenum func, int write);

static void fullscreen_pass(ID3D11RenderTargetView *target, int w, int h, ID3D11PixelShader *p, ID3D11BlendState *bs) {
    ID3D11DeviceContext_OMSetRenderTargets(ctx, 1, &target, NULL);
    D3D11_VIEWPORT vp = {0, 0, (float)w, (float)h, 0, 1};
    ID3D11DeviceContext_RSSetViewports(ctx, 1, &vp);
    ID3D11DeviceContext_RSSetState(ctx, get_raster(0));
    float bf[4] = {0, 0, 0, 0};
    ID3D11DeviceContext_OMSetBlendState(ctx, bs, bf, 0xFFFFFFFF);
    ID3D11DeviceContext_OMSetDepthStencilState(ctx, get_depth(0, 0x207, 0), 0);
    ID3D11DeviceContext_IASetInputLayout(ctx, NULL);
    ID3D11DeviceContext_IASetPrimitiveTopology(ctx, D3D11_PRIMITIVE_TOPOLOGY_TRIANGLELIST);
    ID3D11DeviceContext_VSSetShader(ctx, down_vs, NULL, 0);
    ID3D11DeviceContext_PSSetShader(ctx, p, NULL, 0);
    ID3D11DeviceContext_PSSetConstantBuffers(ctx, 1, 1, &pbuf);
    ID3D11DeviceContext_Draw(ctx, 3, 0);
}

static void unbind_srvs(void) {
    ID3D11ShaderResourceView *nul[3] = {NULL, NULL, NULL};
    ID3D11DeviceContext_PSSetShaderResources(ctx, 0, 3, nul);
}

// Ambient occlusion into ao_tex, then multiplied onto the scene (blend: dest * src colour).
static void run_ao(void) {
    write_pbuf();
    ID3D11DeviceContext_OMSetRenderTargets(ctx, 0, NULL, NULL); // depth may not stay bound while it is read
    ID3D11DeviceContext_PSSetShaderResources(ctx, 1, 1, &depth_srv);
    fullscreen_pass(ao_rtv, rt_w, rt_h, ao_ps, get_blend(0, 1, 0, 1));
    ID3D11DeviceContext_OMSetRenderTargets(ctx, 0, NULL, NULL);
    unbind_srvs();
    ID3D11DeviceContext_PSSetShaderResources(ctx, 2, 1, &ao_srv);
    fullscreen_pass(rtv, rt_w, rt_h, aoblur_ps, get_blend(1, 0, 0x300, 1));
    unbind_srvs();
}
static ID3D11VertexShader *vs;
static ID3D11PixelShader *ps;
static ID3D11InputLayout *il;
static ID3D11Buffer *cbuf, *vbuf;
static UINT vb_pos;
#define VB_BYTES (16 << 20)

typedef struct { float x, y, z, w, r, g, b, a, u0, v0, u1, v1; } Vtx;
typedef struct {
    float mvp[16], mv[16], tm0[16], tm1[16];
    float fogcolor[4], fogp[4], alpha[4];
    int tex[4];
    float px[4]; // x, y = size of texture 0; z = 1: sharp-bilinear (pixel font); w = 1: alpha-weighted bilinear
    float uvc[4]; // texture 0 coordinates clamped to this rectangle (min xy, max zw) when z > x
} CB;

static const char *k_hlsl =
    "cbuffer C : register(b0) { float4x4 mvp; float4x4 mv; float4x4 tm0; float4x4 tm1;\n"
    "  float4 fogcolor; float4 fogp; float4 alpha; int4 tex; float4 px; float4 uvc; };\n"
    "Texture2D t0 : register(t0); Texture2D t1 : register(t1);\n"
    "SamplerState s0 : register(s0); SamplerState s1 : register(s1);\n"
    "struct VI { float4 p : POSITION; float4 c : COLOR; float4 t : TEXCOORD0; };\n"
    "struct VO { float4 p : SV_Position; float4 c : COLOR; float4 t : TEXCOORD0; float f : FOG; };\n"
    "VO vsmain(VI i) {\n"
    "  VO o; float4 cp = mul(mvp, i.p); cp.z = (cp.z + cp.w) * 0.5; o.p = cp; o.c = i.c;\n"
    "  float4 a = mul(tm0, float4(i.t.xy, 0, 1)); float4 b = mul(tm1, float4(i.t.zw, 0, 1));\n"
    "  o.t = float4(a.xy / a.w, b.xy / b.w);\n"
    "  float ez = abs(mul(mv, i.p).z); float f = 1;\n"
    "  if (fogp.w == 1) f = (fogp.y - ez) / (fogp.y - fogp.x);\n"
    "  else if (fogp.w == 2) f = exp(-fogp.z * ez);\n"
    "  else if (fogp.w == 3) f = exp(-(fogp.z * ez) * (fogp.z * ez));\n"
    "  o.f = saturate(f); return o; }\n"
    "float4 env(int m, float4 p, float4 x) {\n"
    "  if (m == 0x1E01) return x;\n"
    "  if (m == 0x2101) return float4(lerp(p.rgb, x.rgb, x.a), p.a);\n"
    "  if (m == 0x0104) return float4(saturate(p.rgb + x.rgb), p.a * x.a);\n"
    "  return p * x; }\n"
    "float4 psmain(VO i) : SV_Target {\n"
    "  float4 c = i.c;\n"
    "  if (tex.x) { float4 x; if (uvc.z > uvc.x) i.t.xy = clamp(i.t.xy, uvc.xy, uvc.zw);\n"
    "    if (px.z > 0) { float2 t = i.t.xy * px.xy, s = floor(t + 0.5), d = max(fwidth(t), 1e-4);\n"
    "      x = t0.SampleLevel(s0, (s + clamp((t - s) / d, -0.5, 0.5)) / px.xy, 0); }\n"
    "    else if (px.w > 0) { float2 t = i.t.xy * px.xy - 0.5, f = frac(t); int2 p = (int2)floor(t);\n"
    "      float4 a = t0.Load(int3(p, 0)), b = t0.Load(int3(p + int2(1, 0), 0));\n"
    "      float4 c2 = t0.Load(int3(p + int2(0, 1), 0)), d = t0.Load(int3(p + int2(1, 1), 0));\n"
    "      float4 w = float4((1 - f.x) * (1 - f.y), f.x * (1 - f.y), (1 - f.x) * f.y, f.x * f.y);\n"
    "      float al = dot(w, float4(a.a, b.a, c2.a, d.a));\n"
    "      float3 rgb = w.x * a.a * a.rgb + w.y * b.a * b.rgb + w.z * c2.a * c2.rgb + w.w * d.a * d.rgb;\n"
    "      x = al > 0 ? float4(rgb / al, al) : float4(0, 0, 0, 0); }\n"
    "    else x = t0.Sample(s0, i.t.xy);\n"
    "    c = env(tex.z, c, x); }\n"
    "  if (tex.y) c = env(tex.w, c, t1.Sample(s1, i.t.zw));\n"
    "  if (alpha.z > 0) { float a = c.a, r = alpha.x; int fn = (int)alpha.y; bool ok = true;\n"
    "    if (fn == 0) ok = false; else if (fn == 1) ok = a < r; else if (fn == 2) ok = a == r;\n"
    "    else if (fn == 3) ok = a <= r; else if (fn == 4) ok = a > r; else if (fn == 5) ok = a != r;\n"
    "    else if (fn == 6) ok = a >= r;\n"
    "    if (!ok) discard; }\n"
    "  if (fogp.w > 0) c.rgb = lerp(fogcolor.rgb, c.rgb, i.f);\n"
    "  return c; }\n";

typedef HRESULT(WINAPI *PFN_D3DCompile)(LPCVOID, SIZE_T, LPCSTR, const void *, void *, LPCSTR, LPCSTR, UINT, UINT,
                                         ID3DBlob **, ID3DBlob **);

static ID3DBlob *compile_src(PFN_D3DCompile fn, const char *src, const char *entry, const char *target) {
    ID3DBlob *code = NULL, *err = NULL;
    HRESULT hr = fn(src, strlen(src), "gld3d", NULL, NULL, entry, target, 0, 0, &code, &err);
    if (FAILED(hr)) DLOG("shader %s failed: %s", entry, err ? (char *)ID3D10Blob_GetBufferPointer(err) : "?");
    if (err) ID3D10Blob_Release(err);
    return code;
}

static ID3DBlob *compile(PFN_D3DCompile fn, const char *entry, const char *target) {
    ID3DBlob *code = NULL, *err = NULL;
    HRESULT hr = fn(k_hlsl, strlen(k_hlsl), "gld3d", NULL, NULL, entry, target, 0, 0, &code, &err);
    if (FAILED(hr)) DLOG("shader %s failed: %s", entry, err ? (char *)ID3D10Blob_GetBufferPointer(err) : "?");
    if (err) ID3D10Blob_Release(err);
    return code;
}

static int create_device(HWND hwnd) {
    RECT r;
    GetClientRect(hwnd, &r);
    rt_w = r.right > 0 ? r.right : 640;
    rt_h = r.bottom > 0 ? r.bottom : 480;
    if (g_windowed) { rt_w = g_tw; rt_h = g_th; } // render size; Present stretches it to the window
    D3D_FEATURE_LEVEL fl;
    HRESULT hr = D3D11CreateDevice(NULL, D3D_DRIVER_TYPE_HARDWARE, NULL, D3D11_CREATE_DEVICE_BGRA_SUPPORT, NULL, 0,
                                   D3D11_SDK_VERSION, &dev, &fl, &ctx);
    if (FAILED(hr)) { DLOG("D3D11CreateDevice failed %08lx", hr); return 0; }

    IDXGIDevice *xd = NULL; IDXGIAdapter *ad = NULL; IDXGIFactory *fac = NULL;
    ID3D11Device_QueryInterface(dev, &IID_IDXGIDevice, (void **)&xd);
    IDXGIDevice1 *xd1 = NULL;
    if (SUCCEEDED(ID3D11Device_QueryInterface(dev, &IID_IDXGIDevice1, (void **)&xd1))) {
        // Input lag: by default DXGI queues up to three frames ahead of the display.
        IDXGIDevice1_SetMaximumFrameLatency(xd1, g_latency);
        IDXGIDevice1_Release(xd1);
    }
    IDXGIDevice_GetAdapter(xd, &ad);
    IDXGIAdapter_GetParent(ad, &IID_IDXGIFactory, (void **)&fac);
    DXGI_ADAPTER_DESC adesc;
    IDXGIAdapter_GetDesc(ad, &adesc);
    DLOG("adapter %ls, feature level %x, client %dx%d", adesc.Description, fl, rt_w, rt_h);

    DXGI_SWAP_CHAIN_DESC d;
    memset(&d, 0, sizeof d);
    d.BufferDesc.Width = rt_w; d.BufferDesc.Height = rt_h;
    d.BufferDesc.Format = DXGI_FORMAT_B8G8R8A8_UNORM;
    d.SampleDesc.Count = 1;
    d.BufferUsage = DXGI_USAGE_RENDER_TARGET_OUTPUT;
    d.BufferCount = 2;
    d.OutputWindow = hwnd;
    d.Windowed = TRUE;
    d.SwapEffect = (DXGI_SWAP_EFFECT)4; // FLIP_DISCARD
    hr = IDXGIFactory_CreateSwapChain(fac, (IUnknown *)dev, &d, &sc);
    if (FAILED(hr)) {
        DLOG("flip swap chain failed %08lx, falling back to DISCARD", hr);
        d.BufferCount = 1;
        d.SwapEffect = DXGI_SWAP_EFFECT_DISCARD;
        hr = IDXGIFactory_CreateSwapChain(fac, (IUnknown *)dev, &d, &sc);
        if (FAILED(hr)) { DLOG("swap chain failed %08lx", hr); return 0; }
    }
    IDXGIFactory_MakeWindowAssociation(fac, hwnd, DXGI_MWA_NO_ALT_ENTER);
    IDXGIFactory_Release(fac); IDXGIAdapter_Release(ad); IDXGIDevice_Release(xd);

    // Persistent render target: the game redraws only what changes, so the image must survive a present.
    // It is rendered at SuperSample x the client size with MSAA, then resolved / box-filtered into the back buffer.
    out_w = rt_w; out_h = rt_h;
    rt_w = out_w * g_ss; rt_h = out_h * g_ss;
    UINT q = 0;
    while (g_msaa > 1 && (FAILED(ID3D11Device_CheckMultisampleQualityLevels(dev, DXGI_FORMAT_B8G8R8A8_UNORM, g_msaa, &q)) || !q)) g_msaa /= 2;
    if (g_msaa < 1) g_msaa = 1;
    D3D11_TEXTURE2D_DESC td;
    memset(&td, 0, sizeof td);
    td.Width = rt_w; td.Height = rt_h; td.MipLevels = 1; td.ArraySize = 1;
    td.Format = DXGI_FORMAT_B8G8R8A8_UNORM; td.SampleDesc.Count = g_msaa;
    td.Usage = D3D11_USAGE_DEFAULT;
    td.BindFlags = D3D11_BIND_RENDER_TARGET | (g_msaa == 1 && g_ss > 1 ? D3D11_BIND_SHADER_RESOURCE : 0);
    if (FAILED(ID3D11Device_CreateTexture2D(dev, &td, NULL, &rt_tex))) { DLOG("render target %dx%d msaa %d failed", rt_w, rt_h, g_msaa); return 0; }
    ID3D11Device_CreateRenderTargetView(dev, (ID3D11Resource *)rt_tex, NULL, &rtv);
    if (g_msaa > 1 && g_ss > 1) { // resolve target that the downscale pass reads
        td.SampleDesc.Count = 1; td.BindFlags = D3D11_BIND_SHADER_RESOURCE;
        ID3D11Device_CreateTexture2D(dev, &td, NULL, &res_tex);
    }
    if (g_ss > 1) ID3D11Device_CreateShaderResourceView(dev, (ID3D11Resource *)(res_tex ? res_tex : rt_tex), NULL, &src_srv);
    td.SampleDesc.Count = g_msaa;
    // Depth is also read as a texture by the ambient-occlusion pass.
    td.Format = DXGI_FORMAT_R24G8_TYPELESS; td.BindFlags = D3D11_BIND_DEPTH_STENCIL | D3D11_BIND_SHADER_RESOURCE;
    if (FAILED(ID3D11Device_CreateTexture2D(dev, &td, NULL, &ds_tex))) { DLOG("depth buffer failed"); return 0; }
    D3D11_DEPTH_STENCIL_VIEW_DESC dvd;
    memset(&dvd, 0, sizeof dvd);
    dvd.Format = DXGI_FORMAT_D24_UNORM_S8_UINT;
    dvd.ViewDimension = g_msaa > 1 ? D3D11_DSV_DIMENSION_TEXTURE2DMS : D3D11_DSV_DIMENSION_TEXTURE2D;
    ID3D11Device_CreateDepthStencilView(dev, (ID3D11Resource *)ds_tex, &dvd, &dsv);
    D3D11_SHADER_RESOURCE_VIEW_DESC svd;
    memset(&svd, 0, sizeof svd);
    svd.Format = DXGI_FORMAT_R24_UNORM_X8_TYPELESS;
    svd.ViewDimension = g_msaa > 1 ? D3D11_SRV_DIMENSION_TEXTURE2DMS : D3D11_SRV_DIMENSION_TEXTURE2D;
    svd.Texture2D.MipLevels = 1;
    ID3D11Device_CreateShaderResourceView(dev, (ID3D11Resource *)ds_tex, &svd, &depth_srv);
    // Ambient-occlusion buffer (internal size) and the post-processing source (window size).
    D3D11_TEXTURE2D_DESC aod = td;
    aod.SampleDesc.Count = 1; aod.Format = DXGI_FORMAT_R8_UNORM;
    aod.BindFlags = D3D11_BIND_RENDER_TARGET | D3D11_BIND_SHADER_RESOURCE;
    ID3D11Device_CreateTexture2D(dev, &aod, NULL, &ao_tex);
    ID3D11Device_CreateRenderTargetView(dev, (ID3D11Resource *)ao_tex, NULL, &ao_rtv);
    ID3D11Device_CreateShaderResourceView(dev, (ID3D11Resource *)ao_tex, NULL, &ao_srv);
    aod.Width = out_w; aod.Height = out_h; aod.Format = DXGI_FORMAT_B8G8R8A8_UNORM;
    ID3D11Device_CreateTexture2D(dev, &aod, NULL, &post_tex);
    ID3D11Device_CreateRenderTargetView(dev, (ID3D11Resource *)post_tex, NULL, &post_rtv);
    ID3D11Device_CreateShaderResourceView(dev, (ID3D11Resource *)post_tex, NULL, &post_srv);
    DLOG("render target %dx%d (supersample %d, msaa %d) -> %dx%d", rt_w, rt_h, g_ss, g_msaa, out_w, out_h);
    float black[4] = {0, 0, 0, 1};
    ID3D11DeviceContext_ClearRenderTargetView(ctx, rtv, black);
    ID3D11DeviceContext_ClearDepthStencilView(ctx, dsv, D3D11_CLEAR_DEPTH, 1, 0);

    HMODULE dc = LoadLibraryA("d3dcompiler_47.dll");
    PFN_D3DCompile fn = dc ? (PFN_D3DCompile)GetProcAddress(dc, "D3DCompile") : NULL;
    if (!fn) { DLOG("d3dcompiler_47.dll missing"); return 0; }
    ID3DBlob *vb = compile(fn, "vsmain", "vs_4_0"), *pb = compile(fn, "psmain", "ps_4_0");
    if (!vb || !pb) return 0;
    ID3D11Device_CreateVertexShader(dev, ID3D10Blob_GetBufferPointer(vb), ID3D10Blob_GetBufferSize(vb), NULL, &vs);
    ID3D11Device_CreatePixelShader(dev, ID3D10Blob_GetBufferPointer(pb), ID3D10Blob_GetBufferSize(pb), NULL, &ps);
    D3D11_INPUT_ELEMENT_DESC ie[3] = {
        {"POSITION", 0, DXGI_FORMAT_R32G32B32A32_FLOAT, 0, 0, D3D11_INPUT_PER_VERTEX_DATA, 0},
        {"COLOR", 0, DXGI_FORMAT_R32G32B32A32_FLOAT, 0, 16, D3D11_INPUT_PER_VERTEX_DATA, 0},
        {"TEXCOORD", 0, DXGI_FORMAT_R32G32B32A32_FLOAT, 0, 32, D3D11_INPUT_PER_VERTEX_DATA, 0},
    };
    ID3D11Device_CreateInputLayout(dev, ie, 3, ID3D10Blob_GetBufferPointer(vb), ID3D10Blob_GetBufferSize(vb), &il);
    ID3D10Blob_Release(vb); ID3D10Blob_Release(pb);
    {
        char src[8192];
        sprintf(src, "#define SS %d\n#define MS %d\n%s", g_ss, g_msaa, k_post_hlsl);
        ID3DBlob *b[5] = {compile_src(fn, src, "vsq", "vs_4_1"), compile_src(fn, src, "psdown", "ps_4_1"),
                          compile_src(fn, src, "pspost", "ps_4_1"), compile_src(fn, src, "psao", "ps_4_1"),
                          compile_src(fn, src, "psaoblur", "ps_4_1")};
        for (int i = 0; i < 5; i++) if (!b[i]) return 0;
        ID3D11Device_CreateVertexShader(dev, ID3D10Blob_GetBufferPointer(b[0]), ID3D10Blob_GetBufferSize(b[0]), NULL, &down_vs);
        ID3D11PixelShader **pss[4] = {&down_ps, &post_ps, &ao_ps, &aoblur_ps};
        for (int i = 0; i < 4; i++)
            ID3D11Device_CreatePixelShader(dev, ID3D10Blob_GetBufferPointer(b[i + 1]), ID3D10Blob_GetBufferSize(b[i + 1]), NULL, pss[i]);
        for (int i = 0; i < 5; i++) ID3D10Blob_Release(b[i]);
        D3D11_BUFFER_DESC pd;
        memset(&pd, 0, sizeof pd);
        pd.ByteWidth = 48; pd.Usage = D3D11_USAGE_DYNAMIC;
        pd.BindFlags = D3D11_BIND_CONSTANT_BUFFER; pd.CPUAccessFlags = D3D11_CPU_ACCESS_WRITE;
        ID3D11Device_CreateBuffer(dev, &pd, NULL, &pbuf);
    }

    D3D11_BUFFER_DESC bd;
    memset(&bd, 0, sizeof bd);
    bd.ByteWidth = sizeof(CB); bd.Usage = D3D11_USAGE_DYNAMIC;
    bd.BindFlags = D3D11_BIND_CONSTANT_BUFFER; bd.CPUAccessFlags = D3D11_CPU_ACCESS_WRITE;
    ID3D11Device_CreateBuffer(dev, &bd, NULL, &cbuf);
    bd.ByteWidth = VB_BYTES; bd.BindFlags = D3D11_BIND_VERTEX_BUFFER;
    ID3D11Device_CreateBuffer(dev, &bd, NULL, &vbuf);
    DLOG("device ready");
    return 1;
}

// ---------------------------------------------------------------- state objects (cached)
static D3D11_BLEND blend_map(GLenum f, int alpha_slot) {
    switch (f) {
    case 0: return D3D11_BLEND_ZERO;
    case 1: return D3D11_BLEND_ONE;
    case 0x300: return alpha_slot ? D3D11_BLEND_SRC_ALPHA : D3D11_BLEND_SRC_COLOR;
    case 0x301: return alpha_slot ? D3D11_BLEND_INV_SRC_ALPHA : D3D11_BLEND_INV_SRC_COLOR;
    case 0x302: return D3D11_BLEND_SRC_ALPHA;
    case 0x303: return D3D11_BLEND_INV_SRC_ALPHA;
    case 0x304: return D3D11_BLEND_ONE;  // DST_ALPHA: the GL framebuffer had no alpha channel
    case 0x305: return D3D11_BLEND_ZERO; // ONE_MINUS_DST_ALPHA
    case 0x306: return alpha_slot ? D3D11_BLEND_DEST_ALPHA : D3D11_BLEND_DEST_COLOR;
    case 0x307: return alpha_slot ? D3D11_BLEND_INV_DEST_ALPHA : D3D11_BLEND_INV_DEST_COLOR;
    case 0x308: return D3D11_BLEND_SRC_ALPHA_SAT;
    }
    return D3D11_BLEND_ONE;
}

#define CACHE 64
static struct { unsigned key; ID3D11BlendState *s; } bl_c[CACHE];
static struct { unsigned key; ID3D11DepthStencilState *s; } ds_c[CACHE];
static struct { unsigned key; ID3D11SamplerState *s; } sm_c[CACHE];
static ID3D11RasterizerState *rs_c[2];

static ID3D11BlendState *get_blend(int on, GLenum src, GLenum dst, int write_color) {
    unsigned key = 1u | (on ? 2u : 0) | (write_color ? 4u : 0) | ((src & 0xFFF) << 4) | ((dst & 0xFFF) << 16);
    for (int i = 0; i < CACHE && bl_c[i].key; i++)
        if (bl_c[i].key == key) return bl_c[i].s;
    D3D11_BLEND_DESC d;
    memset(&d, 0, sizeof d);
    D3D11_RENDER_TARGET_BLEND_DESC *t = &d.RenderTarget[0];
    t->BlendEnable = on;
    t->SrcBlend = blend_map(src, 0); t->DestBlend = blend_map(dst, 0); t->BlendOp = D3D11_BLEND_OP_ADD;
    t->SrcBlendAlpha = blend_map(src, 1); t->DestBlendAlpha = blend_map(dst, 1); t->BlendOpAlpha = D3D11_BLEND_OP_ADD;
    t->RenderTargetWriteMask = write_color ? D3D11_COLOR_WRITE_ENABLE_ALL : 0;
    ID3D11BlendState *s = NULL;
    ID3D11Device_CreateBlendState(dev, &d, &s);
    for (int i = 0; i < CACHE; i++)
        if (!bl_c[i].key) { bl_c[i].key = key; bl_c[i].s = s; break; }
    return s;
}

static ID3D11DepthStencilState *get_depth(int on, GLenum func, int write) {
    unsigned key = 1u | (on ? 2u : 0) | (write ? 4u : 0) | ((func & 0xF) << 4);
    for (int i = 0; i < CACHE && ds_c[i].key; i++)
        if (ds_c[i].key == key) return ds_c[i].s;
    D3D11_DEPTH_STENCIL_DESC d;
    memset(&d, 0, sizeof d);
    d.DepthEnable = on;
    d.DepthWriteMask = write ? D3D11_DEPTH_WRITE_MASK_ALL : D3D11_DEPTH_WRITE_MASK_ZERO;
    d.DepthFunc = (D3D11_COMPARISON_FUNC)((func & 7) + 1);
    ID3D11DepthStencilState *s = NULL;
    ID3D11Device_CreateDepthStencilState(dev, &d, &s);
    for (int i = 0; i < CACHE; i++)
        if (!ds_c[i].key) { ds_c[i].key = key; ds_c[i].s = s; break; }
    return s;
}

static int g_aniso = 8;
static ID3D11SamplerState *get_sampler(GLenum minf, GLenum magf, GLenum ws, GLenum wt) {
    int mip = minf >= 0x2700 && minf <= 0x2703;
    int minlin = minf == 0x2601 || minf == 0x2701 || minf == 0x2703;
    int miplin = minf == 0x2702 || minf == 0x2703;
    int maglin = magf == 0x2601;
    int clamp_s = ws != 0x2901, clamp_t = wt != 0x2901;
    unsigned key = 1u | (mip << 1) | (minlin << 2) | (miplin << 3) | (maglin << 4) | (clamp_s << 5) | (clamp_t << 6);
    for (int i = 0; i < CACHE && sm_c[i].key; i++)
        if (sm_c[i].key == key) return sm_c[i].s;
    D3D11_SAMPLER_DESC d;
    memset(&d, 0, sizeof d);
    d.Filter = (D3D11_FILTER)((minlin ? 0x10 : 0) | (maglin ? 0x4 : 0) | (miplin ? 0x1 : 0));
    if (mip && minlin && maglin && g_aniso > 1) { d.Filter = D3D11_FILTER_ANISOTROPIC; d.MaxAnisotropy = g_aniso; }
    d.AddressU = clamp_s ? D3D11_TEXTURE_ADDRESS_CLAMP : D3D11_TEXTURE_ADDRESS_WRAP;
    d.AddressV = clamp_t ? D3D11_TEXTURE_ADDRESS_CLAMP : D3D11_TEXTURE_ADDRESS_WRAP;
    d.AddressW = D3D11_TEXTURE_ADDRESS_CLAMP;
    d.ComparisonFunc = D3D11_COMPARISON_NEVER;
    d.MinLOD = 0;
    d.MaxLOD = mip ? D3D11_FLOAT32_MAX : 0;
    ID3D11SamplerState *s = NULL;
    ID3D11Device_CreateSamplerState(dev, &d, &s);
    for (int i = 0; i < CACHE; i++)
        if (!sm_c[i].key) { sm_c[i].key = key; sm_c[i].s = s; break; }
    return s;
}

static ID3D11RasterizerState *get_raster(int scissor) {
    if (!rs_c[scissor]) {
        D3D11_RASTERIZER_DESC d;
        memset(&d, 0, sizeof d);
        d.FillMode = D3D11_FILL_SOLID; d.CullMode = D3D11_CULL_NONE;
        d.DepthClipEnable = TRUE; d.ScissorEnable = scissor;
        ID3D11Device_CreateRasterizerState(dev, &d, &rs_c[scissor]);
    }
    return rs_c[scissor];
}

// ---------------------------------------------------------------- texture pack (dump / high-resolution replacement)
// Every texture is identified by an FNV-1a hash of its size and level-0 pixels.
// TextureDump=1 writes textures\dump\<hash>.png; textures\hd\<hash>.dds (BC7 etc. with mips) replaces the original.
static int g_tex_dump, g_tex_replace = 1;
static char g_tex_dir[MAX_PATH], g_tex_pack[64] = "hd"; // TexturePack: sub-folder of textures\ with the DDS files

static unsigned long long tex_hash(const Tex *t) {
    unsigned long long h = 1469598103934665603ull;
    int wh[2] = {t->w[0], t->h[0]};
    const BYTE *p = (const BYTE *)wh;
    for (int i = 0; i < 8; i++) h = (h ^ p[i]) * 1099511628211ull;
    p = t->lv[0];
    for (size_t i = 0, n = (size_t)t->w[0] * t->h[0] * 4; i < n; i++) h = (h ^ p[i]) * 1099511628211ull;
    return h;
}

static unsigned crc_tab[256];
static unsigned crc32(unsigned c, const BYTE *b, size_t n) {
    if (!crc_tab[1])
        for (unsigned i = 0; i < 256; i++) {
            unsigned k = i;
            for (int j = 0; j < 8; j++) k = k & 1 ? 0xEDB88320u ^ (k >> 1) : k >> 1;
            crc_tab[i] = k;
        }
    c ^= 0xFFFFFFFFu;
    while (n--) c = crc_tab[(c ^ *b++) & 255] ^ (c >> 8);
    return c ^ 0xFFFFFFFFu;
}
static void be32(BYTE *p, unsigned v) { p[0] = v >> 24; p[1] = v >> 16; p[2] = v >> 8; p[3] = (BYTE)v; }
static void png_chunk(FILE *f, const char *type, const BYTE *data, unsigned n) {
    BYTE h[8];
    be32(h, n); memcpy(h + 4, type, 4);
    fwrite(h, 1, 8, f);
    if (n) fwrite(data, 1, n, f);
    unsigned c = crc32(0, (const BYTE *)type, 4);
    c = crc32(c, data, n);
    be32(h, c);
    fwrite(h, 1, 4, f);
}
// Uncompressed (stored-deflate) RGBA PNG: small code, the upscaler reads it like any other PNG.
static void write_png(const char *name, const BYTE *rgba, int w, int h) {
    size_t raw = (size_t)(w * 4 + 1) * h, blocks = (raw + 65534) / 65535;
    size_t zn = 2 + raw + blocks * 5 + 4;
    BYTE *z = malloc(zn), *r = malloc(raw);
    if (!z || !r) { free(z); free(r); return; }
    for (int y = 0; y < h; y++) { r[(size_t)y * (w * 4 + 1)] = 0; memcpy(r + (size_t)y * (w * 4 + 1) + 1, rgba + (size_t)y * w * 4, w * 4); }
    size_t o = 0;
    z[o++] = 0x78; z[o++] = 0x01;
    unsigned a = 1, b = 0;
    for (size_t i = 0; i < raw; i++) { a = (a + r[i]) % 65521; b = (b + a) % 65521; }
    for (size_t pos = 0; pos < raw; pos += 65535) {
        unsigned len = (unsigned)(raw - pos > 65535 ? 65535 : raw - pos);
        z[o++] = pos + len >= raw;
        z[o++] = len & 255; z[o++] = len >> 8; z[o++] = ~len & 255; z[o++] = (~len >> 8) & 255;
        memcpy(z + o, r + pos, len); o += len;
    }
    be32(z + o, (b << 16) | a); o += 4;
    FILE *f = fopen(name, "wb");
    if (f) {
        static const BYTE sig[8] = {0x89, 'P', 'N', 'G', 13, 10, 26, 10};
        BYTE ih[13];
        be32(ih, w); be32(ih + 4, h); ih[8] = 8; ih[9] = 6; ih[10] = ih[11] = ih[12] = 0;
        fwrite(sig, 1, 8, f);
        png_chunk(f, "IHDR", ih, 13);
        png_chunk(f, "IDAT", z, (unsigned)o);
        png_chunk(f, "IEND", NULL, 0);
        fclose(f);
    }
    free(z); free(r);
}

typedef struct HdTex { unsigned long long hash; ID3D11Texture2D *t; ID3D11ShaderResourceView *srv; int levels; struct HdTex *next; } HdTex;
static HdTex *g_hd;
static int g_hd_loaded, g_hd_missing;

static int dds_block_bytes(DXGI_FORMAT f) {
    switch (f) {
    case DXGI_FORMAT_BC1_UNORM: case DXGI_FORMAT_BC4_UNORM: return 8;
    case DXGI_FORMAT_BC2_UNORM: case DXGI_FORMAT_BC3_UNORM: case DXGI_FORMAT_BC5_UNORM: case DXGI_FORMAT_BC7_UNORM: return 16;
    default: return 0;
    }
}

// Loads textures\hd\<hash>.dds once; returns NULL when there is none.
static HdTex *hd_get_in(const char *pack, unsigned long long hash) {
    for (HdTex *h = g_hd; h; h = h->next)
        if (h->hash == hash) return h->srv ? h : NULL;
    HdTex *h = calloc(1, sizeof *h);
    h->hash = hash; h->next = g_hd; g_hd = h;
    char name[MAX_PATH];
    sprintf(name, "%s\\%s\\%016llx.dds", g_tex_dir, pack, hash);
    FILE *f = fopen(name, "rb");
    if (!f && strcmp(pack, "hd") && strcmp(pack, "font")) { // a texture without a 16x file still gets its 4x one
        sprintf(name, "%s\\hd\\%016llx.dds", g_tex_dir, hash);
        f = fopen(name, "rb");
    }
    if (!f) { g_hd_missing++; return NULL; }
    fseek(f, 0, SEEK_END);
    long sz = ftell(f);
    fseek(f, 0, SEEK_SET);
    BYTE *b = malloc(sz);
    if (!b || fread(b, 1, sz, f) != (size_t)sz || sz < 128 || memcmp(b, "DDS ", 4)) { fclose(f); free(b); return NULL; }
    fclose(f);
    UINT hgt = *(UINT *)(b + 12), wid = *(UINT *)(b + 16), mips = *(UINT *)(b + 28);
    UINT fourcc = *(UINT *)(b + 84), off = 128;
    DXGI_FORMAT fmt = DXGI_FORMAT_UNKNOWN;
    if (fourcc == 0x30315844 /* DX10 */) { fmt = *(DXGI_FORMAT *)(b + 128); off = 148; }
    else if (fourcc == 0x31545844) fmt = DXGI_FORMAT_BC1_UNORM;  // DXT1
    else if (fourcc == 0x33545844) fmt = DXGI_FORMAT_BC2_UNORM;  // DXT3
    else if (fourcc == 0x35545844) fmt = DXGI_FORMAT_BC3_UNORM;  // DXT5
    else fmt = DXGI_FORMAT_B8G8R8A8_UNORM;                        // uncompressed 32 bit
    if (fmt == DXGI_FORMAT_BC7_UNORM_SRGB) fmt = DXGI_FORMAT_BC7_UNORM;
    if (fmt == DXGI_FORMAT_R8G8B8A8_UNORM_SRGB) fmt = DXGI_FORMAT_R8G8B8A8_UNORM;
    if (fmt == DXGI_FORMAT_B8G8R8A8_UNORM_SRGB) fmt = DXGI_FORMAT_B8G8R8A8_UNORM;
    if (!mips) mips = 1;
    if (mips > 16) mips = 16;
    int bb = dds_block_bytes(fmt);
    D3D11_SUBRESOURCE_DATA init[16];
    UINT w = wid, hh = hgt, pos = off;
    for (UINT i = 0; i < mips; i++) {
        UINT pitch = bb ? ((w + 3) / 4) * bb : w * 4, rows = bb ? (hh + 3) / 4 : hh;
        if (pos + pitch * rows > (UINT)sz) { mips = i; break; }
        init[i].pSysMem = b + pos; init[i].SysMemPitch = pitch; init[i].SysMemSlicePitch = 0;
        pos += pitch * rows;
        w = w > 1 ? w / 2 : 1; hh = hh > 1 ? hh / 2 : 1;
    }
    D3D11_TEXTURE2D_DESC d;
    memset(&d, 0, sizeof d);
    d.Width = wid; d.Height = hgt; d.MipLevels = mips; d.ArraySize = 1; d.Format = fmt;
    d.SampleDesc.Count = 1; d.Usage = D3D11_USAGE_IMMUTABLE; d.BindFlags = D3D11_BIND_SHADER_RESOURCE;
    if (mips && SUCCEEDED(ID3D11Device_CreateTexture2D(dev, &d, init, &h->t))) {
        ID3D11Device_CreateShaderResourceView(dev, (ID3D11Resource *)h->t, NULL, &h->srv);
        h->levels = mips;
        g_hd_loaded++;
    } else DLOG("hd texture %s: format %d %ux%u not loadable", name, fmt, wid, hgt);
    free(b);
    return h->srv ? h : NULL;
}

static HdTex *hd_get(unsigned long long hash) { return hd_get_in(g_tex_pack, hash); }

// Creates or refreshes the GPU copy; returns NULL for an incomplete texture (GL then disables the unit).
static ID3D11ShaderResourceView *tex_srv(Tex *t) {
    if (!t || !t->lv[0]) return NULL;
    if (t->dirty || !t->srv) {
        tex_release_gpu(t);
        int n = 0, w0 = t->w[0], h0 = t->h[0];
        while (n < MAXL && t->lv[n]) {
            int ew = w0 >> n, eh = h0 >> n;
            if (ew < 1) ew = 1;
            if (eh < 1) eh = 1;
            if (t->w[n] != ew || t->h[n] != eh) break;
            n++;
            if (ew == 1 && eh == 1) break;
        }
        t->replaced = 0;
        if ((g_tex_dump || g_tex_replace) && w0 >= 8 && h0 >= 8 && !t->dynamic) {
            unsigned long long hash = tex_hash(t);
            if (g_tex_dump) {
                char name[MAX_PATH];
                sprintf(name, "%s\\dump\\%016llx.png", g_tex_dir, hash);
                if (GetFileAttributesA(name) == INVALID_FILE_ATTRIBUTES) write_png(name, t->lv[0], w0, h0);
            }
            HdTex *hd = g_tex_replace ? hd_get(hash) : NULL;
            if (hd) {
                t->t = hd->t; t->srv = hd->srv;
                ID3D11Texture2D_AddRef(t->t); ID3D11ShaderResourceView_AddRef(t->srv);
                t->nlev = 0; // no sub-image updates into the replacement
                t->replaced = 1;
                t->dirty = 0;
                return t->srv;
            }
        }
        D3D11_TEXTURE2D_DESC d;
        memset(&d, 0, sizeof d);
        d.Width = w0; d.Height = h0; d.MipLevels = n; d.ArraySize = 1;
        d.Format = DXGI_FORMAT_R8G8B8A8_UNORM; d.SampleDesc.Count = 1;
        d.Usage = D3D11_USAGE_DEFAULT; d.BindFlags = D3D11_BIND_SHADER_RESOURCE;
        D3D11_SUBRESOURCE_DATA init[MAXL];
        for (int i = 0; i < n; i++) { init[i].pSysMem = t->lv[i]; init[i].SysMemPitch = t->w[i] * 4; init[i].SysMemSlicePitch = 0; }
        if (FAILED(ID3D11Device_CreateTexture2D(dev, &d, init, &t->t))) { DLOG("texture %u %dx%d failed", t->name, w0, h0); return NULL; }
        ID3D11Device_CreateShaderResourceView(dev, (ID3D11Resource *)t->t, NULL, &t->srv);
        t->nlev = n;
        t->dirty = 0;
    }
    return t->srv;
}

// ---------------------------------------------------------------- game space -> render target
// 2D (orthographic) content keeps the game's aspect, centred; a full-screen perspective pass fills the target.
static int proj_is_persp(void) { return pj_st[pj_top].m[11] != 0; }
static int wide_pass(void) {
    return proj_is_persp() && S.vx == 0 && S.vy == 0 && S.vw == game_w() && S.vh == game_h();
}
static void map_rect(int x, int y, int w, int h, int wide, float *o) { // o = GL-style x, y(bottom), w, h
    float gw = (float)game_w(), gh = (float)game_h();
    // A game resolution within 1 % of the target aspect (852x480 vs 16:9) fills the target without bars.
    float ar = (gw / gh) / ((float)rt_w / rt_h);
    if (ar > 0.99f && ar < 1.01f) wide = 1;
    if (wide) {
        float kx = rt_w / gw, ky = rt_h / gh;
        o[0] = x * kx; o[1] = y * ky; o[2] = w * kx; o[3] = h * ky;
        return;
    }
    float s = rt_w / gw < rt_h / gh ? rt_w / gw : rt_h / gh;
    float ox = (rt_w - gw * s) * 0.5f, oy = (rt_h - gh * s) * 0.5f;
    o[0] = ox + x * s; o[1] = oy + y * s; o[2] = w * s; o[3] = h * s;
}

static void set_view(int wide) {
    float r[4];
    map_rect(S.vx, S.vy, S.vw, S.vh, wide, r);
    D3D11_VIEWPORT vp = {r[0], rt_h - (r[1] + r[3]), r[2], r[3], 0, 1};
    ID3D11DeviceContext_RSSetViewports(ctx, 1, &vp);
    if (S.scissor_on) {
        map_rect(S.sx, S.sy, S.sw, S.sh, wide, r);
        D3D11_RECT sr = {(LONG)floorf(r[0] + 0.5f), (LONG)floorf(rt_h - (r[1] + r[3]) + 0.5f),
                         (LONG)floorf(r[0] + r[2] + 0.5f), (LONG)floorf(rt_h - r[1] + 0.5f)};
        ID3D11DeviceContext_RSSetScissorRects(ctx, 1, &sr);
    }
}

// ---------------------------------------------------------------- drawing
static void upload_and_draw(D3D11_PRIMITIVE_TOPOLOGY topo, const Vtx *v, int n) {
    UINT bytes = n * sizeof(Vtx);
    if (bytes > VB_BYTES) { DLOG("batch too large: %d vertices", n); return; }
    D3D11_MAPPED_SUBRESOURCE m;
    D3D11_MAP mode = D3D11_MAP_WRITE_NO_OVERWRITE;
    if (vb_pos + bytes > VB_BYTES) { vb_pos = 0; mode = D3D11_MAP_WRITE_DISCARD; }
    if (FAILED(ID3D11DeviceContext_Map(ctx, (ID3D11Resource *)vbuf, 0, mode, 0, &m))) return;
    memcpy((BYTE *)m.pData + vb_pos, v, bytes);
    ID3D11DeviceContext_Unmap(ctx, (ID3D11Resource *)vbuf, 0);
    UINT stride = sizeof(Vtx), off = vb_pos;
    ID3D11DeviceContext_IASetVertexBuffers(ctx, 0, 1, &vbuf, &stride, &off);
    ID3D11DeviceContext_IASetPrimitiveTopology(ctx, topo);
    ID3D11DeviceContext_Draw(ctx, n, 0);
    vb_pos += bytes;
}

static void common_pipeline(void) {
    ID3D11DeviceContext_OMSetRenderTargets(ctx, 1, &rtv, dsv);
    ID3D11DeviceContext_IASetInputLayout(ctx, il);
    ID3D11DeviceContext_VSSetShader(ctx, vs, NULL, 0);
    ID3D11DeviceContext_PSSetShader(ctx, ps, NULL, 0);
    ID3D11DeviceContext_VSSetConstantBuffers(ctx, 0, 1, &cbuf);
    ID3D11DeviceContext_PSSetConstantBuffers(ctx, 0, 1, &cbuf);
}

static void write_cb(const CB *c) {
    D3D11_MAPPED_SUBRESOURCE m;
    if (SUCCEEDED(ID3D11DeviceContext_Map(ctx, (ID3D11Resource *)cbuf, 0, D3D11_MAP_WRITE_DISCARD, 0, &m))) {
        memcpy(m.pData, c, sizeof *c);
        ID3D11DeviceContext_Unmap(ctx, (ID3D11Resource *)cbuf, 0);
    }
}

// Diagnostics ("set texlog N"): logs the next N 2D draws with texture, filter and game-space rectangle,
// and writes each texture they use once to shots\tex_<name>.png in the game directory.
static int g_texlog;
static int g_sharp_font = 1, g_font_replace = 1, g_uv_clamp = 1, g_log_unrep;
static void texlog_draw(const M4 *mvp, const Vtx *v, int n) {
    g_texlog--;
    float x0 = 1e9f, y0 = 1e9f, x1 = -1e9f, y1 = -1e9f, u0 = 1e9f, v0 = 1e9f, u1 = -1e9f, v1 = -1e9f;
    for (int i = 0; i < n; i++) {
        const float *m = mvp->m;
        float cx = m[0] * v[i].x + m[4] * v[i].y + m[8] * v[i].z + m[12] * v[i].w;
        float cy = m[1] * v[i].x + m[5] * v[i].y + m[9] * v[i].z + m[13] * v[i].w;
        float cw = m[3] * v[i].x + m[7] * v[i].y + m[11] * v[i].z + m[15] * v[i].w;
        if (cw == 0) cw = 1;
        float gx = S.vx + (cx / cw * 0.5f + 0.5f) * S.vw, gy = S.vy + (cy / cw * 0.5f + 0.5f) * S.vh;
        if (gx < x0) x0 = gx; if (gx > x1) x1 = gx; if (gy < y0) y0 = gy; if (gy > y1) y1 = gy;
        if (v[i].u0 < u0) u0 = v[i].u0; if (v[i].u0 > u1) u1 = v[i].u0;
        if (v[i].v0 < v0) v0 = v[i].v0; if (v[i].v0 > v1) v1 = v[i].v0;
    }
    Tex *t = S.tex_on[0] ? tex_get(S.bound[0], 0) : NULL;
    DLOG("2d n=%d game=(%.1f,%.1f)-(%.1f,%.1f) tex=%u %dx%d dyn=%d rep=%d min=%x mag=%x uv=(%.4f,%.4f)-(%.4f,%.4f) blend=%d alpha=%d col=%.2f,%.2f,%.2f,%.2f",
         n, x0, y0, x1, y1, t ? t->name : 0, t ? t->w[0] : 0, t ? t->h[0] : 0, t ? t->dynamic : 0, t ? t->replaced : 0,
         t ? t->minf : 0, t ? t->magf : 0, u0, v0, u1, v1, S.blend_on, S.alpha_on, v[0].r, v[0].g, v[0].b, v[0].a);
    if (t && t->lv[0]) {
        char name[MAX_PATH];
        CreateDirectoryA("shots", NULL);
        sprintf(name, "shots\\tex_%u.png", t->name);
        if (GetFileAttributesA(name) == INVALID_FILE_ATTRIBUTES) write_png(name, t->lv[0], t->w[0], t->h[0]);
    }
}

// Pictures the game composes into a big dynamic texture and draws as a part of it (menu titles, "LADE ..."
// and other text strips) go through the texture pack like whole textures: the drawn sub-rectangle is
// hashed (size and pixels, as tex_hash), dumped as textures\dump\<hash>.png with TextureDump=1 and replaced
// by <pack>\<hash>.dds when that exists. A small cache keeps the hash until the texture changes.
// TextureDump also lists the hashes of everything drawn in 2D (menus, HUD) in textures\ui_textures.txt:
// the pack build upscales those with a model made for drawn art instead of the photo model.
static void ui_list(unsigned long long hash) {
    static unsigned long long seen[4096];
    static int n;
    for (int i = 0; i < n; i++)
        if (seen[i] == hash) return;
    if (n < 4096) seen[n++] = hash;
    char name[MAX_PATH];
    sprintf(name, "%s\\ui_textures.txt", g_tex_dir);
    FILE *f = fopen(name, "a");
    if (f) { fprintf(f, "%016llx\n", hash); fclose(f); }
}

typedef struct { GLuint name; unsigned gen; int x, y, w, h; HdTex *hd; } SubRect;
static SubRect g_sub[64];
static int g_sub_next;
static Vtx *g_subv;
static int g_subv_cap;
static unsigned long long rect_hash(const Tex *t, int x, int y, int w, int h) {
    unsigned long long hh = 1469598103934665603ull;
    int wh[2] = {w, h};
    const BYTE *p = (const BYTE *)wh;
    for (int i = 0; i < 8; i++) hh = (hh ^ p[i]) * 1099511628211ull;
    for (int r = 0; r < h; r++) {
        p = t->lv[0] + ((size_t)(y + r) * t->w[0] + x) * 4;
        for (int i = 0; i < w * 4; i++) hh = (hh ^ p[i]) * 1099511628211ull;
    }
    return hh;
}
// Returns the replacement's vertices (uv spread over the whole replacement) and its view, or NULL.
static const Vtx *subrect_replace(Tex *t, const Vtx *v, int n, ID3D11ShaderResourceView **srv) {
    float u0 = 1e9f, v0 = 1e9f, u1 = -1e9f, v1 = -1e9f;
    for (int i = 0; i < n; i++) {
        u0 = fminf(u0, v[i].u0); u1 = fmaxf(u1, v[i].u0);
        v0 = fminf(v0, v[i].v0); v1 = fmaxf(v1, v[i].v0);
    }
    int W = t->w[0], H = t->h[0];
    int x = (int)floorf(u0 * W + 0.5f), y = (int)floorf(v0 * H + 0.5f);
    int w = (int)floorf(u1 * W + 0.5f) - x, h = (int)floorf(v1 * H + 0.5f) - y;
    if (x < 0 || y < 0 || w < 16 || h < 16 || x + w > W || y + h > H) return NULL;
    SubRect *s = NULL;
    for (int i = 0; i < 64; i++)
        if (g_sub[i].name == t->name && g_sub[i].gen == t->gen && g_sub[i].x == x && g_sub[i].y == y && g_sub[i].w == w && g_sub[i].h == h) { s = &g_sub[i]; break; }
    if (!s) {
        s = &g_sub[g_sub_next++ & 63];
        s->name = t->name; s->gen = t->gen; s->x = x; s->y = y; s->w = w; s->h = h;
        unsigned long long hash = rect_hash(t, x, y, w, h);
        if (g_tex_dump) ui_list(hash);
        if (g_tex_dump) {
            char name[MAX_PATH];
            sprintf(name, "%s\\dump\\%016llx.png", g_tex_dir, hash);
            if (GetFileAttributesA(name) == INVALID_FILE_ATTRIBUTES) {
                BYTE *buf = malloc((size_t)w * h * 4);
                if (buf) {
                    for (int r = 0; r < h; r++) memcpy(buf + (size_t)r * w * 4, t->lv[0] + ((size_t)(y + r) * W + x) * 4, (size_t)w * 4);
                    write_png(name, buf, w, h);
                    free(buf);
                }
            }
        }
        s->hd = g_tex_replace ? hd_get(hash) : NULL;
    }
    if (!s->hd) return NULL;
    if (n > g_subv_cap) { g_subv_cap = n * 2; g_subv = realloc(g_subv, g_subv_cap * sizeof(Vtx)); }
    memcpy(g_subv, v, n * sizeof(Vtx));
    for (int i = 0; i < n; i++) {
        g_subv[i].u0 = (v[i].u0 - u0) / (u1 - u0);
        g_subv[i].v0 = (v[i].v0 - v0) / (v1 - v0);
    }
    *srv = s->hd->srv;
    return g_subv;
}

// Tall font atlas: each 1/16 x 1/16 glyph cell of the game's atlas sits in the middle half of a cell twice
// as high, so every glyph quad (6 vertices) is drawn at twice its height around its centre with the uv
// widened to the whole tall cell. Accents and descenders then have room without changing the layout.
static Vtx *g_fontv;
static int g_fontv_cap;
static const Vtx *font_tall_quads(const Vtx *v, int n) {
    if (n % 6) return v;
    if (n > g_fontv_cap) { g_fontv_cap = n * 2; g_fontv = realloc(g_fontv, g_fontv_cap * sizeof(Vtx)); }
    memcpy(g_fontv, v, n * sizeof(Vtx));
    for (int q = 0; q < n; q += 6) {
        Vtx *g = g_fontv + q;
        float v0 = 1e9f, v1 = -1e9f, y0 = 1e9f, y1 = -1e9f, u0 = 1e9f, u1 = -1e9f;
        for (int i = 0; i < 6; i++) {
            v0 = fminf(v0, g[i].v0); v1 = fmaxf(v1, g[i].v0);
            u0 = fminf(u0, g[i].u0); u1 = fmaxf(u1, g[i].u0);
            y0 = fminf(y0, g[i].y); y1 = fmaxf(y1, g[i].y);
        }
        if (fabsf(v1 - v0 - 0.0625f) > 0.001f || fabsf(u1 - u0 - 0.0625f) > 0.001f) continue; // not one glyph cell
        float row = floorf(v0 * 16 + 0.5f), cy = (y0 + y1) * 0.5f;
        for (int i = 0; i < 6; i++) {
            float t = (g[i].v0 * 16 - row - 0.5f) * 2 + 0.5f; // -0.5 .. 1.5 across the tall cell
            g[i].v0 = (row * 128 + 32 + t * 64) / 2048;
            g[i].y = cy + (g[i].y - cy) * 2;
        }
    }
    return g_fontv;
}

static void draw_list(D3D11_PRIMITIVE_TOPOLOGY topo, const Vtx *v, int n) {
    if (!dev || n <= 0) return;
    if (g_ao_on && !g_ao_done && g_frame_has3d && g_frame_cleared && !proj_is_persp()) {
        run_ao(); // the 3D scene of this frame is complete, the HUD is about to be drawn
        g_ao_done = 1;
    }
    common_pipeline();
    CB c;
    memset(&c, 0, sizeof c);
    int wide = wide_pass();
    M4 p = pj_st[pj_top], mvp;
    if (wide) {
        // Keep the vertical field of view and widen the horizontal one to the target's aspect.
        float k = ((float)rt_w / rt_h) / ((float)game_w() / game_h());
        p.m[0] /= k; p.m[4] /= k; p.m[8] /= k; p.m[12] /= k;
    }
    m_mul(&mvp, &p, &mv_st[mv_top]);
    if (proj_is_persp()) g_ao_proj = p;
    if (proj_is_persp() && !g_frame_has3d && n >= 3) { // first 3D draw of the frame: matrix and vertex positions
        g_frame_has3d = 1;
        g_frame_mvp = mvp;
        unsigned h = 2166136261u;
        for (int i = 0; i < n && i < 64; i++) {
            const BYTE *b = (const BYTE *)&v[i];
            for (int k = 0; k < 16; k++) h = (h ^ b[k]) * 16777619u;
        }
        g_frame_vhash = h;
    }
    memcpy(c.mvp, mvp.m, 64);
    memcpy(c.mv, mv_st[mv_top].m, 64);
    memcpy(c.tm0, tx_st[0][tx_top[0]].m, 64);
    memcpy(c.tm1, tx_st[1][tx_top[1]].m, 64);
    memcpy(c.fogcolor, S.fogcolor, 16);
    c.fogp[0] = S.fogstart; c.fogp[1] = S.fogend; c.fogp[2] = S.fogdens;
    c.fogp[3] = !S.fog_on ? 0 : S.fogmode == 0x2601 ? 1 : S.fogmode == 0x801 ? 3 : 2;
    c.alpha[0] = S.aref; c.alpha[1] = (float)(S.afunc & 7); c.alpha[2] = S.alpha_on ? 1.f : 0.f;
    ID3D11ShaderResourceView *srv[2] = {NULL, NULL};
    ID3D11SamplerState *smp[2] = {NULL, NULL};
    for (int u = 0; u < 2; u++) {
        if (!S.tex_on[u]) continue;
        Tex *t = tex_get(S.bound[u], 0);
        srv[u] = tex_srv(t);
        if (!srv[u]) continue;
        smp[u] = get_sampler(t->minf, t->magf, t->ws, t->wt);
        c.tex[u] = 1;
        c.tex[2 + u] = S.env[u];
        // The menu/HUD pixel font: an 8x8-cell 128x128 atlas the game fills with glTexSubImage2D. Once its
        // pixels hash to a file in textures/font, that atlas is drawn instead (trilinear, it is minified);
        // otherwise the 8x8 glyphs get sharp-bilinear sampling.
        if (u == 0 && t->dynamic && t->w[0] == 128 && t->h[0] == 128 && !proj_is_persp()) {
            if (t->font_gen != t->gen) {
                t->font_gen = t->gen;
                unsigned long long h = tex_hash(t);
                t->font_hd = hd_get_in("font", h);
                t->font_tall = 0;
                if (t->font_hd) {
                    D3D11_TEXTURE2D_DESC fd;
                    ID3D11Texture2D_GetDesc(t->font_hd->t, &fd);
                    t->font_tall = fd.Height == 2 * fd.Width;
                }
                static int logged;
                if (logged++ < 20) DLOG("font atlas %u: hash %016llx, replacement %s", t->name, h, t->font_hd ? "loaded" : "none");
            }
            if (g_font_replace && t->font_hd) {
                srv[u] = t->font_hd->srv;
                smp[u] = get_sampler(0x2703, 0x2601, t->ws, t->wt);
                if (t->font_tall && topo == D3D11_PRIMITIVE_TOPOLOGY_TRIANGLELIST) v = font_tall_quads(v, n);
            } else if (g_sharp_font) {
                c.px[0] = 128; c.px[1] = 128; c.px[2] = 1;
            }
        }
    }
    // A single 2D quad that shows part of a texture without tiling (menu pictures in a corner of a big
    // dynamic texture, tabs, banners) samples only inside that part: bilinear magnification otherwise
    // reaches half a texel past it into wrapped-around or stale texels, which shows as lines at the edges.
    // A sub-rectangle of a big dynamic texture with its own pack entry: drawn from the replacement
    // (trilinear and clamped: the high-resolution picture is minified).
    int sub_replaced = 0;
    if ((g_tex_dump || g_tex_replace) && c.tex[0] && !c.px[2] && n == 6 && !proj_is_persp() &&
        topo == D3D11_PRIMITIVE_TOPOLOGY_TRIANGLELIST) {
        Tex *t = tex_get(S.bound[0], 0);
        if (t && t->dynamic && t->lv[0] && !(t->font_hd && g_font_replace) && t->w[0] >= 16 && t->h[0] >= 16) {
            const Vtx *nv = subrect_replace(t, v, n, &srv[0]);
            if (nv) {
                v = nv;
                smp[0] = get_sampler(0x2703, 0x2601, 0x812F, 0x812F);
                sub_replaced = 1;
            }
        }
    }
    if (g_uv_clamp && c.tex[0] && !c.px[2] && !sub_replaced && n == 6 && !proj_is_persp()) {
        Tex *t = tex_get(S.bound[0], 0);
        const float *tm = tx_st[0][tx_top[0]].m;
        if (t && !(t->font_hd && g_font_replace) && tm[0] == 1 && tm[5] == 1 && tm[12] == 0 && tm[13] == 0 && tm[1] == 0 && tm[4] == 0) {
            float u0 = 1e9f, v0 = 1e9f, u1 = -1e9f, v1 = -1e9f;
            for (int i = 0; i < n; i++) {
                u0 = fminf(u0, v[i].u0); u1 = fmaxf(u1, v[i].u0);
                v0 = fminf(v0, v[i].v0); v1 = fmaxf(v1, v[i].v0);
            }
            float hu = 0.5f / t->w[0], hv = 0.5f / t->h[0];
            if (u0 >= -1e-4f && v0 >= -1e-4f && u1 <= 1.0001f && v1 <= 1.0001f && u1 - u0 > 2 * hu && v1 - v0 > 2 * hv) {
                c.uvc[0] = u0 + hu; c.uvc[1] = v0 + hv; c.uvc[2] = u1 - hu; c.uvc[3] = v1 - hv;
                // Transparent texels keep their (colour-key) colour: weight the bilinear taps by alpha so it
                // does not bleed into the edge as a pink or dark seam. Level 0 of the texture actually bound.
                if (S.blend_on && t->magf == 0x2601 && srv[0] && !t->replaced) {
                    ID3D11Resource *res = NULL;
                    ID3D11ShaderResourceView_GetResource(srv[0], &res);
                    D3D11_TEXTURE2D_DESC td;
                    ID3D11Texture2D_GetDesc((ID3D11Texture2D *)res, &td);
                    ID3D11Resource_Release(res);
                    c.px[0] = (float)td.Width; c.px[1] = (float)td.Height; c.px[3] = 1;
                    float ku = 0.5f / td.Width, kv = 0.5f / td.Height;
                    c.uvc[0] = u0 + ku; c.uvc[2] = u1 - ku;
                    c.uvc[1] = v0 + kv; c.uvc[3] = v1 - kv;
                }
            }
        }
    }
    if (g_tex_dump && c.tex[0] && !proj_is_persp()) {
        Tex *t = tex_get(S.bound[0], 0);
        if (t && !t->dynamic && !t->ui_listed && t->lv[0] && t->w[0] >= 8 && t->h[0] >= 8) { t->ui_listed = 1; ui_list(tex_hash(t)); }
    }
    if (g_log_unrep && c.tex[0] && !proj_is_persp() && !sub_replaced) {
        Tex *t = tex_get(S.bound[0], 0);
        if (t && t->lv[0] && !t->replaced && !(t->font_hd && g_font_replace) && t->unrep_logged < 4) {
            float x0 = 1e9f, x1 = -1e9f, u0 = 1e9f, u1 = -1e9f;
            for (int i = 0; i < n; i++) { x0 = fminf(x0, v[i].x); x1 = fmaxf(x1, v[i].x); u0 = fminf(u0, v[i].u0); u1 = fmaxf(u1, v[i].u0); }
            t->unrep_logged++;
            DLOG("unreplaced 2D: tex %u %dx%d dyn %d hash %016llx n %d uv-width %.3f quad-width %.1f", t->name, t->w[0], t->h[0],
                 t->dynamic, t->dynamic ? 0ull : tex_hash(t), n, u1 - u0, x1 - x0);
        }
    }
    if (g_texlog > 0 && !proj_is_persp()) texlog_draw(&mvp, v, n);
    write_cb(&c);
    ID3D11DeviceContext_PSSetShaderResources(ctx, 0, 2, srv);
    ID3D11DeviceContext_PSSetSamplers(ctx, 0, 2, smp);
    float bf[4] = {0, 0, 0, 0};
    ID3D11DeviceContext_OMSetBlendState(ctx, get_blend(S.blend_on, S.bsrc, S.bdst, 1), bf, 0xFFFFFFFF);
    ID3D11DeviceContext_OMSetDepthStencilState(ctx, get_depth(S.depth_on, S.dfunc, S.dmask), 0);
    ID3D11DeviceContext_RSSetState(ctx, get_raster(S.scissor_on));
    set_view(wide);
    upload_and_draw(topo, v, n);
}

// Any GL primitive -> triangle or line list.
static Vtx *g_tmp;
static int g_tmp_cap;
static Vtx *tmp_need(int n) {
    if (n > g_tmp_cap) { g_tmp_cap = n * 2; g_tmp = realloc(g_tmp, g_tmp_cap * sizeof(Vtx)); }
    return g_tmp;
}

static void draw_prims(GLenum mode, const Vtx *v, int n) {
    int k = 0;
    Vtx *o;
    switch (mode) {
    case 0: draw_list(D3D11_PRIMITIVE_TOPOLOGY_POINTLIST, v, n); return;
    case 1: draw_list(D3D11_PRIMITIVE_TOPOLOGY_LINELIST, v, n & ~1); return;
    case 3: case 2: // LINE_STRIP, LINE_LOOP
        if (n < 2) return;
        o = tmp_need(n * 2 + 2);
        for (int i = 0; i + 1 < n; i++) { o[k++] = v[i]; o[k++] = v[i + 1]; }
        if (mode == 2) { o[k++] = v[n - 1]; o[k++] = v[0]; }
        draw_list(D3D11_PRIMITIVE_TOPOLOGY_LINELIST, o, k);
        return;
    case 4: draw_list(D3D11_PRIMITIVE_TOPOLOGY_TRIANGLELIST, v, n - n % 3); return;
    case 5: draw_list(D3D11_PRIMITIVE_TOPOLOGY_TRIANGLESTRIP, v, n); return;
    case 6: case 9: // TRIANGLE_FAN, POLYGON
        if (n < 3) return;
        o = tmp_need((n - 2) * 3);
        for (int i = 1; i + 1 < n; i++) { o[k++] = v[0]; o[k++] = v[i]; o[k++] = v[i + 1]; }
        break;
    case 7: // QUADS
        o = tmp_need(n / 4 * 6);
        for (int i = 0; i + 3 < n; i += 4) {
            o[k++] = v[i]; o[k++] = v[i + 1]; o[k++] = v[i + 2];
            o[k++] = v[i]; o[k++] = v[i + 2]; o[k++] = v[i + 3];
        }
        break;
    case 8: // QUAD_STRIP
        if (n < 4) return;
        o = tmp_need((n / 2 - 1) * 6);
        for (int i = 0; i + 3 < n; i += 2) {
            o[k++] = v[i]; o[k++] = v[i + 1]; o[k++] = v[i + 3];
            o[k++] = v[i]; o[k++] = v[i + 3]; o[k++] = v[i + 2];
        }
        break;
    default: return;
    }
    draw_list(D3D11_PRIMITIVE_TOPOLOGY_TRIANGLELIST, o, k);
}

// ---------------------------------------------------------------- immediate mode and vertex arrays
static Vtx *im;
static int im_n, im_cap, in_begin;
static GLenum im_mode;

static void im_push(float x, float y, float z, float w) {
    if (im_n == im_cap) { im_cap = im_cap ? im_cap * 2 : 1024; im = realloc(im, im_cap * sizeof(Vtx)); }
    Vtx *v = &im[im_n++];
    v->x = x; v->y = y; v->z = z; v->w = w;
    v->r = S.color[0]; v->g = S.color[1]; v->b = S.color[2]; v->a = S.color[3];
    v->u0 = S.tc[0][0]; v->v0 = S.tc[0][1]; v->u1 = S.tc[1][0]; v->v1 = S.tc[1][1];
}

static float arr_f(const Arr *a, int i, int c) {
    int csz = a->type == 0x140A ? 8 : (a->type == 0x1402 || a->type == 0x1403) ? 2 : a->type == 0x1401 || a->type == 0x1400 ? 1 : 4;
    const BYTE *p = a->ptr + (size_t)i * (a->stride ? a->stride : a->size * csz) + c * csz;
    switch (a->type) {
    case 0x1406: return *(const float *)p;
    case 0x140A: return (float)*(const double *)p;
    case 0x1402: return *(const short *)p;
    case 0x1404: return (float)*(const int *)p;
    case 0x1401: return *p / 255.f;
    case 0x1403: return *(const unsigned short *)p / 65535.f;
    }
    return 0;
}

static void fetch(int i, Vtx *v) {
    const Arr *a = &S.va;
    v->x = arr_f(a, i, 0); v->y = arr_f(a, i, 1);
    v->z = a->size > 2 ? arr_f(a, i, 2) : 0;
    v->w = a->size > 3 ? arr_f(a, i, 3) : 1;
    if (S.ca.on) {
        v->r = arr_f(&S.ca, i, 0); v->g = arr_f(&S.ca, i, 1); v->b = arr_f(&S.ca, i, 2);
        v->a = S.ca.size > 3 ? arr_f(&S.ca, i, 3) : 1;
    } else { v->r = S.color[0]; v->g = S.color[1]; v->b = S.color[2]; v->a = S.color[3]; }
    float *uv[2][2] = {{&v->u0, &v->v0}, {&v->u1, &v->v1}};
    for (int u = 0; u < 2; u++) {
        if (S.ta[u].on) { *uv[u][0] = arr_f(&S.ta[u], i, 0); *uv[u][1] = arr_f(&S.ta[u], i, 1); }
        else { *uv[u][0] = S.tc[u][0]; *uv[u][1] = S.tc[u][1]; }
    }
}

// ---------------------------------------------------------------- display lists
typedef struct { int op; double a[6]; } Cmd;
typedef struct { Cmd *c; int n, cap; } List;
static List *lists;
static int lists_cap;
static GLuint list_next = 1, list_comp;
static GLenum list_mode;

enum { OP_BEGIN = 1, OP_END, OP_V2F, OP_V2I, OP_V3F, OP_TC2F, OP_C4F, OP_C4UB, OP_TRANSF, OP_TRANSD, OP_BIND,
       OP_ENABLE, OP_DISABLE, OP_BLEND, OP_PUSH, OP_POP, OP_IDENT, OP_MATMODE, OP_RECTI, OP_CALL, OP_ALPHA,
       OP_DMASK, OP_DFUNC, OP_TEXENV, OP_TEXPARAM, OP_SHADE, OP_SCALEF, OP_ROTF };

static List *list_get(GLuint id, int create) {
    if (id >= (GLuint)lists_cap) {
        if (!create) return NULL;
        int nc = (int)id + 256;
        lists = realloc(lists, nc * sizeof(List));
        memset(lists + lists_cap, 0, (nc - lists_cap) * sizeof(List));
        lists_cap = nc;
    }
    return &lists[id];
}

// Records the call while a list is compiled; returns 1 when the call must not execute (GL_COMPILE).
static int rec(int op, double a0, double a1, double a2, double a3, double a4, double a5) {
    if (!list_comp) return 0;
    List *l = list_get(list_comp, 1);
    if (l->n == l->cap) { l->cap = l->cap ? l->cap * 2 : 16; l->c = realloc(l->c, l->cap * sizeof(Cmd)); }
    Cmd *c = &l->c[l->n++];
    c->op = op;
    c->a[0] = a0; c->a[1] = a1; c->a[2] = a2; c->a[3] = a3; c->a[4] = a4; c->a[5] = a5;
    return list_mode == 0x1300;
}
#define REC(op, ...) if (rec(op, __VA_ARGS__)) return
static void unlisted(const char *name) {
    if (list_comp) DLOG("call inside display list not recorded: %s", name);
}

// ---------------------------------------------------------------- GL entry points
GLF(void, glBegin, (GLenum m)) { REC(OP_BEGIN, m, 0, 0, 0, 0, 0); in_begin = 1; im_mode = m; im_n = 0; }
GLF(void, glEnd, (void)) {
    REC(OP_END, 0, 0, 0, 0, 0, 0);
    in_begin = 0;
    draw_prims(im_mode, im, im_n);
    im_n = 0;
}
GLF(void, glVertex2f, (GLfloat x, GLfloat y)) { REC(OP_V2F, x, y, 0, 0, 0, 0); im_push(x, y, 0, 1); }
GLF(void, glVertex2i, (GLint x, GLint y)) { REC(OP_V2I, x, y, 0, 0, 0, 0); im_push((float)x, (float)y, 0, 1); }
GLF(void, glVertex3f, (GLfloat x, GLfloat y, GLfloat z)) { REC(OP_V3F, x, y, z, 0, 0, 0); im_push(x, y, z, 1); }
GLF(void, glVertex3fv, (const GLfloat *v)) { glVertex3f(v[0], v[1], v[2]); }
GLF(void, glTexCoord2f, (GLfloat s, GLfloat t)) { REC(OP_TC2F, s, t, 0, 0, 0, 0); S.tc[0][0] = s; S.tc[0][1] = t; }
GLF(void, glMultiTexCoord2fARB, (GLenum u, GLfloat s, GLfloat t)) { int i = (u - 0x84C0) & 1; S.tc[i][0] = s; S.tc[i][1] = t; }
GLF(void, glColor4f, (GLfloat r, GLfloat g, GLfloat b, GLfloat a)) {
    REC(OP_C4F, r, g, b, a, 0, 0);
    S.color[0] = r; S.color[1] = g; S.color[2] = b; S.color[3] = a;
}
GLF(void, glColor4ub, (GLubyte r, GLubyte g, GLubyte b, GLubyte a)) {
    REC(OP_C4UB, r, g, b, a, 0, 0);
    S.color[0] = r / 255.f; S.color[1] = g / 255.f; S.color[2] = b / 255.f; S.color[3] = a / 255.f;
}
GLF(void, glColor3f, (GLfloat r, GLfloat g, GLfloat b)) { glColor4f(r, g, b, 1); }

GLF(void, glArrayElement, (GLint i)) {
    unlisted("glArrayElement");
    Vtx v;
    fetch(i, &v);
    im_push(v.x, v.y, v.z, v.w);
    Vtx *p = &im[im_n - 1];
    p->r = v.r; p->g = v.g; p->b = v.b; p->a = v.a; p->u0 = v.u0; p->v0 = v.v0; p->u1 = v.u1; p->v1 = v.v1;
}

GLF(void, glDrawElements, (GLenum mode, GLsizei n, GLenum type, const GLvoid *idx)) {
    unlisted("glDrawElements");
    if (n <= 0 || !S.va.on) return;
    static Vtx *buf;
    static int cap;
    if (n > cap) { cap = n * 2; buf = realloc(buf, cap * sizeof(Vtx)); }
    for (int i = 0; i < n; i++) {
        int k = type == 0x1401 ? ((const GLubyte *)idx)[i] : type == 0x1403 ? ((const unsigned short *)idx)[i] : (int)((const GLuint *)idx)[i];
        fetch(k, &buf[i]);
    }
    draw_prims(mode, buf, n);
}

GLF(void, glVertexPointer, (GLint s, GLenum t, GLsizei st, const GLvoid *p)) { S.va.size = s; S.va.type = t; S.va.stride = st; S.va.ptr = p; }
GLF(void, glColorPointer, (GLint s, GLenum t, GLsizei st, const GLvoid *p)) { S.ca.size = s; S.ca.type = t; S.ca.stride = st; S.ca.ptr = p; }
GLF(void, glTexCoordPointer, (GLint s, GLenum t, GLsizei st, const GLvoid *p)) {
    Arr *a = &S.ta[S.cunit];
    a->size = s; a->type = t; a->stride = st; a->ptr = p;
}
static Arr *client_arr(GLenum c) {
    if (c == 0x8074) return &S.va;
    if (c == 0x8076) return &S.ca;
    if (c == 0x8078) return &S.ta[S.cunit];
    return NULL;
}
GLF(void, glEnableClientState, (GLenum c)) { Arr *a = client_arr(c); if (a) a->on = 1; }
GLF(void, glDisableClientState, (GLenum c)) { Arr *a = client_arr(c); if (a) a->on = 0; }
GLF(void, glActiveTextureARB, (GLenum u)) { S.unit = (u - 0x84C0) & 1; }
GLF(void, glClientActiveTextureARB, (GLenum u)) { S.cunit = (u - 0x84C0) & 1; }

static void set_cap(GLenum c, int on) {
    switch (c) {
    case 0x0BE2: S.blend_on = on; break;
    case 0x0BC0: S.alpha_on = on; break;
    case 0x0B71: S.depth_on = on; break;
    case 0x0C11: S.scissor_on = on; break;
    case 0x0B60: S.fog_on = on; break;
    case 0x0DE1: S.tex_on[S.unit] = on; break;
    default: break; // lighting, cull face, dither: not used by the game in a way that matters here
    }
}
GLF(void, glEnable, (GLenum c)) { REC(OP_ENABLE, c, 0, 0, 0, 0, 0); set_cap(c, 1); }
GLF(void, glDisable, (GLenum c)) { REC(OP_DISABLE, c, 0, 0, 0, 0, 0); set_cap(c, 0); }
GLF(GLboolean, glIsEnabled, (GLenum c)) {
    switch (c) {
    case 0x0BE2: return S.blend_on; case 0x0BC0: return S.alpha_on; case 0x0B71: return S.depth_on;
    case 0x0C11: return S.scissor_on; case 0x0B60: return S.fog_on; case 0x0DE1: return S.tex_on[S.unit];
    }
    return 0;
}

GLF(void, glBlendFunc, (GLenum s, GLenum d)) { REC(OP_BLEND, s, d, 0, 0, 0, 0); S.bsrc = s; S.bdst = d; }
GLF(void, glAlphaFunc, (GLenum f, GLclampf r)) { REC(OP_ALPHA, f, r, 0, 0, 0, 0); S.afunc = f; S.aref = r; }
GLF(void, glDepthFunc, (GLenum f)) { REC(OP_DFUNC, f, 0, 0, 0, 0, 0); S.dfunc = f; }
GLF(void, glDepthMask, (GLboolean m)) { REC(OP_DMASK, m, 0, 0, 0, 0, 0); S.dmask = m != 0; }
GLF(void, glShadeModel, (GLenum m)) { REC(OP_SHADE, m, 0, 0, 0, 0, 0); }
GLF(void, glHint, (GLenum t, GLenum m)) {}
GLF(void, glDrawBuffer, (GLenum m)) {}
GLF(void, glReadBuffer, (GLenum m)) {}
GLF(void, glFlush, (void)) {}
GLF(void, glFinish, (void)) {}
GLF(GLenum, glGetError, (void)) { return 0; }
GLF(void, glClearColor, (GLclampf r, GLclampf g, GLclampf b, GLclampf a)) {
    S.clearcolor[0] = r; S.clearcolor[1] = g; S.clearcolor[2] = b; S.clearcolor[3] = a;
}
GLF(void, glFogf, (GLenum p, GLfloat v)) {
    if (p == 0x0B62) S.fogdens = v; else if (p == 0x0B63) S.fogstart = v; else if (p == 0x0B64) S.fogend = v;
    else if (p == 0x0B65) S.fogmode = (GLenum)v;
}
GLF(void, glFogi, (GLenum p, GLint v)) {
    if (p == 0x0B65) S.fogmode = v; else glFogf(p, (GLfloat)v);
}
GLF(void, glFogfv, (GLenum p, const GLfloat *v)) {
    if (p == 0x0B66) memcpy(S.fogcolor, v, 16); else glFogf(p, v[0]);
}
GLF(void, glTexEnvi, (GLenum t, GLenum p, GLint v)) {
    REC(OP_TEXENV, t, p, v, 0, 0, 0);
    if (p == 0x2200) S.env[S.unit] = v;
}
GLF(void, glTexEnvf, (GLenum t, GLenum p, GLfloat v)) { glTexEnvi(t, p, (GLint)v); }
GLF(void, glPixelStorei, (GLenum p, GLint v)) {
    if (p >= 0x0CF0 && p <= 0x0CF5) S.unpack[p - 0x0CF0] = v;
    else if (p >= 0x0D00 && p <= 0x0D05) S.pack[p - 0x0D00] = v;
}
GLF(void, glPixelTransferf, (GLenum p, GLfloat v)) {
    static const GLenum sc[4] = {0x0D14, 0x0D18, 0x0D1A, 0x0D1C}, bi[4] = {0x0D15, 0x0D19, 0x0D1B, 0x0D1D};
    for (int i = 0; i < 4; i++) { if (p == sc[i]) S.tscale[i] = v; if (p == bi[i]) S.tbias[i] = v; }
}

// ---- matrices
GLF(void, glMatrixMode, (GLenum m)) { REC(OP_MATMODE, m, 0, 0, 0, 0, 0); g_matmode = m; }
GLF(void, glLoadIdentity, (void)) { REC(OP_IDENT, 0, 0, 0, 0, 0, 0); m_ident(cur_mat()); }
GLF(void, glPushMatrix, (void)) {
    REC(OP_PUSH, 0, 0, 0, 0, 0, 0);
    if (g_matmode == 0x1701) { if (pj_top < 7) { pj_st[pj_top + 1] = pj_st[pj_top]; pj_top++; } }
    else if (g_matmode == 0x1702) { int u = S.unit; if (tx_top[u] < 7) { tx_st[u][tx_top[u] + 1] = tx_st[u][tx_top[u]]; tx_top[u]++; } }
    else if (mv_top < 31) { mv_st[mv_top + 1] = mv_st[mv_top]; mv_top++; }
}
GLF(void, glPopMatrix, (void)) {
    REC(OP_POP, 0, 0, 0, 0, 0, 0);
    if (g_matmode == 0x1701) { if (pj_top) pj_top--; }
    else if (g_matmode == 0x1702) { if (tx_top[S.unit]) tx_top[S.unit]--; }
    else if (mv_top) mv_top--;
}
static void mult(const M4 *b) { M4 *c = cur_mat(); m_mul(c, c, b); }
GLF(void, glMultMatrixf, (const GLfloat *m)) { M4 b; memcpy(b.m, m, 64); mult(&b); }
GLF(void, glMultMatrixd, (const GLdouble *m)) { M4 b; for (int i = 0; i < 16; i++) b.m[i] = (float)m[i]; mult(&b); }
GLF(void, glLoadMatrixf, (const GLfloat *m)) { memcpy(cur_mat()->m, m, 64); }
GLF(void, glLoadMatrixd, (const GLdouble *m)) { for (int i = 0; i < 16; i++) cur_mat()->m[i] = (float)m[i]; }
GLF(void, glTranslatef, (GLfloat x, GLfloat y, GLfloat z)) {
    REC(OP_TRANSF, x, y, z, 0, 0, 0);
    M4 *c = cur_mat();
    for (int r = 0; r < 4; r++) c->m[12 + r] += c->m[r] * x + c->m[4 + r] * y + c->m[8 + r] * z;
}
GLF(void, glTranslated, (GLdouble x, GLdouble y, GLdouble z)) { REC(OP_TRANSD, x, y, z, 0, 0, 0); glTranslatef((float)x, (float)y, (float)z); }
GLF(void, glScalef, (GLfloat x, GLfloat y, GLfloat z)) {
    REC(OP_SCALEF, x, y, z, 0, 0, 0);
    M4 *c = cur_mat();
    for (int r = 0; r < 4; r++) { c->m[r] *= x; c->m[4 + r] *= y; c->m[8 + r] *= z; }
}
GLF(void, glRotatef, (GLfloat a, GLfloat x, GLfloat y, GLfloat z)) {
    REC(OP_ROTF, a, x, y, z, 0, 0);
    float l = sqrtf(x * x + y * y + z * z);
    if (l == 0) return;
    x /= l; y /= l; z /= l;
    float c = cosf(a * 3.14159265f / 180), s = sinf(a * 3.14159265f / 180), t = 1 - c;
    M4 r;
    m_ident(&r);
    r.m[0] = x * x * t + c;     r.m[4] = x * y * t - z * s; r.m[8] = x * z * t + y * s;
    r.m[1] = y * x * t + z * s; r.m[5] = y * y * t + c;     r.m[9] = y * z * t - x * s;
    r.m[2] = z * x * t - y * s; r.m[6] = z * y * t + x * s; r.m[10] = z * z * t + c;
    mult(&r);
}
GLF(void, glFrustum, (GLdouble l, GLdouble r, GLdouble b, GLdouble t, GLdouble n, GLdouble f)) {
    M4 m;
    memset(&m, 0, sizeof m);
    m.m[0] = (float)(2 * n / (r - l)); m.m[5] = (float)(2 * n / (t - b));
    m.m[8] = (float)((r + l) / (r - l)); m.m[9] = (float)((t + b) / (t - b));
    m.m[10] = (float)(-(f + n) / (f - n)); m.m[11] = -1; m.m[14] = (float)(-2 * f * n / (f - n));
    mult(&m);
}
GLF(void, glOrtho, (GLdouble l, GLdouble r, GLdouble b, GLdouble t, GLdouble n, GLdouble f)) {
    M4 m;
    m_ident(&m);
    m.m[0] = (float)(2 / (r - l)); m.m[5] = (float)(2 / (t - b)); m.m[10] = (float)(-2 / (f - n));
    m.m[12] = (float)(-(r + l) / (r - l)); m.m[13] = (float)(-(t + b) / (t - b)); m.m[14] = (float)(-(f + n) / (f - n));
    mult(&m);
}
GLF(void, glViewport, (GLint x, GLint y, GLsizei w, GLsizei h)) { S.vx = x; S.vy = y; S.vw = w; S.vh = h; }
GLF(void, glScissor, (GLint x, GLint y, GLsizei w, GLsizei h)) { S.sx = x; S.sy = y; S.sw = w; S.sh = h; }

GLF(void, glRecti, (GLint x1, GLint y1, GLint x2, GLint y2)) {
    REC(OP_RECTI, x1, y1, x2, y2, 0, 0);
    int n0 = im_n;
    im_push((float)x1, (float)y1, 0, 1); im_push((float)x2, (float)y1, 0, 1);
    im_push((float)x2, (float)y2, 0, 1); im_push((float)x1, (float)y2, 0, 1);
    draw_prims(7, im + n0, 4);
    im_n = n0;
}

// Clear honours the scissor box like GL; a full-screen clear also clears the pillarbox bars.
GLF(void, glClear, (GLbitfield mask)) {
    if (!dev) return;
    int full = !S.scissor_on || (S.sx <= 0 && S.sy <= 0 && S.sx + S.sw >= game_w() && S.sy + S.sh >= game_h());
    if (full) {
        if (mask & 0x0100) g_frame_cleared = 1; // full redraw follows: ambient occlusion will not accumulate
        if (mask & 0x4000) ID3D11DeviceContext_ClearRenderTargetView(ctx, rtv, S.clearcolor);
        if (mask & 0x0100) ID3D11DeviceContext_ClearDepthStencilView(ctx, dsv, D3D11_CLEAR_DEPTH, 1, 0);
        return;
    }
    // Partial clear: a quad over the mapped scissor box, depth written at 1.0.
    common_pipeline();
    CB c;
    memset(&c, 0, sizeof c);
    M4 id;
    m_ident(&id);
    memcpy(c.mvp, id.m, 64); memcpy(c.mv, id.m, 64); memcpy(c.tm0, id.m, 64); memcpy(c.tm1, id.m, 64);
    write_cb(&c);
    ID3D11ShaderResourceView *nul[2] = {NULL, NULL};
    ID3D11DeviceContext_PSSetShaderResources(ctx, 0, 2, nul);
    float bf[4] = {0, 0, 0, 0};
    ID3D11DeviceContext_OMSetBlendState(ctx, get_blend(0, 1, 0, (mask & 0x4000) != 0), bf, 0xFFFFFFFF);
    ID3D11DeviceContext_OMSetDepthStencilState(ctx, get_depth(1, 0x207, (mask & 0x0100) != 0), 0);
    ID3D11DeviceContext_RSSetState(ctx, get_raster(0));
    int wide = wide_pass();
    float r[4];
    map_rect(S.sx, S.sy, S.sw, S.sh, wide, r);
    D3D11_VIEWPORT vp = {r[0], rt_h - (r[1] + r[3]), r[2], r[3], 0, 1};
    ID3D11DeviceContext_RSSetViewports(ctx, 1, &vp);
    float *k = S.clearcolor;
    Vtx q[4] = {{-1, -1, 1, 1, k[0], k[1], k[2], k[3]}, {1, -1, 1, 1, k[0], k[1], k[2], k[3]},
                {-1, 1, 1, 1, k[0], k[1], k[2], k[3]}, {1, 1, 1, 1, k[0], k[1], k[2], k[3]}};
    upload_and_draw(D3D11_PRIMITIVE_TOPOLOGY_TRIANGLESTRIP, q, 4);
}

// ---- textures
GLF(void, glBindTexture, (GLenum t, GLuint n)) { REC(OP_BIND, t, n, 0, 0, 0, 0); S.bound[S.unit] = n; tex_get(n, 1); }
GLF(void, glGenTextures, (GLsizei n, GLuint *out)) {
    static GLuint next = 0x40000000;
    for (int i = 0; i < n; i++) out[i] = next++;
}
GLF(void, glDeleteTextures, (GLsizei n, const GLuint *t)) {
    for (int i = 0; i < n; i++) {
        tex_free(t[i]);
        if (S.bound[0] == t[i]) S.bound[0] = 0;
        if (S.bound[1] == t[i]) S.bound[1] = 0;
    }
}
GLF(void, glTexParameteri, (GLenum tg, GLenum p, GLint v)) {
    REC(OP_TEXPARAM, tg, p, v, 0, 0, 0);
    Tex *t = tex_get(S.bound[S.unit], 1);
    if (p == 0x2801) t->minf = v; else if (p == 0x2800) t->magf = v;
    else if (p == 0x2802) t->ws = v; else if (p == 0x2803) t->wt = v;
}
GLF(void, glTexParameterf, (GLenum tg, GLenum p, GLfloat v)) { glTexParameteri(tg, p, (GLint)v); }

GLF(void, glTexImage2D, (GLenum tg, GLint lvl, GLint ifmt, GLsizei w, GLsizei h, GLint border, GLenum fmt, GLenum type,
                         const GLvoid *px)) {
    unlisted("glTexImage2D");
    if (tg != 0x0DE1 || lvl < 0 || lvl >= MAXL || w <= 0 || h <= 0) return;
    if (lvl == 0 && w * h >= 256 * 256) {
        // Large picture uploads with the exe's return-address chain (frame pointers), to locate who shows them.
        char b[256];
        int n = sprintf(b, "big texture %dx%d at %lu ms, callers:", w, h, GetTickCount());
        // The exe has no frame pointers: scan the stack for values that point just behind a call in .text.
        DWORD *sp = __builtin_frame_address(0);
        int found = 0;
        for (int i = 0; i < 1024 && found < 10 && !IsBadReadPtr(sp + i, 4); i++) {
            DWORD r = sp[i];
            if (r < 0x401006 || r >= 0x649000) continue;
            const BYTE *c = (const BYTE *)(ULONG_PTR)r;
            if (c[-5] == 0xE8 || (c[-6] == 0xFF && c[-5] == 0x15) || (c[-2] == 0xFF && (c[-1] & 0xF8) == 0xD0)) {
                n += sprintf(b + n, " %08lx", r);
                found++;
            }
        }
        DLOG("%s", b);
    }
    Tex *t = tex_get(S.bound[S.unit], 1);
    BYTE *d = realloc(t->lv[lvl], (size_t)w * h * 4);
    if (!d) return;
    if (px) unpack_rect(px, w, h, fmt, type, d, w * 4);
    else memset(d, 0, (size_t)w * h * 4);
    t->lv[lvl] = d; t->w[lvl] = w; t->h[lvl] = h;
    t->gen++;
    t->dirty = 1;
    // Texture pack dump at upload time, so a venue's textures are captured when it loads, not only when seen.
    if (g_tex_dump && px && lvl == 0 && w >= 8 && h >= 8) {
        char name[MAX_PATH];
        sprintf(name, "%s\\dump\\%016llx.png", g_tex_dir, tex_hash(t));
        if (GetFileAttributesA(name) == INVALID_FILE_ATTRIBUTES) write_png(name, d, w, h);
    }
}

GLF(void, glTexSubImage2D, (GLenum tg, GLint lvl, GLint x, GLint y, GLsizei w, GLsizei h, GLenum fmt, GLenum type,
                            const GLvoid *px)) {
    unlisted("glTexSubImage2D");
    Tex *t = tex_get(S.bound[S.unit], 0);
    if (!t || lvl < 0 || lvl >= MAXL || !t->lv[lvl] || w <= 0 || h <= 0) return;
    if (x < 0 || y < 0 || x + w > t->w[lvl] || y + h > t->h[lvl]) return;
    BYTE *dst = t->lv[lvl] + ((size_t)y * t->w[lvl] + x) * 4;
    unpack_rect(px, w, h, fmt, type, dst, t->w[lvl] * 4);
    t->gen++;
    if (t->replaced) t->dirty = 1; // back to the original pixels, which are now animated
    t->dynamic = 1;
    if (t->t && !t->dirty && lvl < t->nlev) {
        D3D11_BOX b = {(UINT)x, (UINT)y, 0, (UINT)(x + w), (UINT)(y + h), 1};
        ID3D11DeviceContext_UpdateSubresource(ctx, (ID3D11Resource *)t->t, lvl, &b, dst, t->w[lvl] * 4, 0);
    } else t->dirty = 1;
}

GLF(void, glGetTexImage, (GLenum tg, GLint lvl, GLenum fmt, GLenum type, GLvoid *out)) {
    Tex *t = tex_get(S.bound[S.unit], 0);
    if (!t || lvl < 0 || lvl >= MAXL || !t->lv[lvl]) return;
    int w = t->w[lvl], h = t->h[lvl], comps = fmt_comps(fmt), ts = type_size(type);
    int rowlen = S.pack[2] > 0 ? S.pack[2] : w, align = S.pack[5] > 0 ? S.pack[5] : 4;
    int psize = comps * ts, dpitch = (rowlen * psize + align - 1) / align * align;
    BYTE *base = (BYTE *)out + (size_t)S.pack[3] * dpitch + (size_t)S.pack[4] * psize;
    for (int y = 0; y < h; y++)
        for (int x = 0; x < w; x++) {
            const BYTE *s = t->lv[lvl] + ((size_t)y * w + x) * 4;
            BYTE *d = base + (size_t)y * dpitch + (size_t)x * psize;
            int ch[4];
            switch (fmt) {
            case 0x80E1: ch[0] = s[2]; ch[1] = s[1]; ch[2] = s[0]; ch[3] = s[3]; break;
            case 0x80E0: ch[0] = s[2]; ch[1] = s[1]; ch[2] = s[0]; break;
            case 0x1909: ch[0] = s[0]; break;
            case 0x190A: ch[0] = s[0]; ch[1] = s[3]; break;
            case 0x1906: ch[0] = s[3]; break;
            default: ch[0] = s[0]; ch[1] = s[1]; ch[2] = s[2]; ch[3] = s[3]; break;
            }
            for (int i = 0; i < comps; i++) {
                if (ts == 2) ((unsigned short *)d)[i] = (unsigned short)(ch[i] * 257);
                else if (ts == 4 && type == 0x1406) ((float *)d)[i] = ch[i] / 255.f;
                else d[i] = (BYTE)ch[i];
            }
        }
}

GLF(void, glGetTexLevelParameteriv, (GLenum tg, GLint lvl, GLenum p, GLint *v)) {
    Tex *t = tex_get(S.bound[S.unit], 0);
    *v = 0;
    if (!t || lvl < 0 || lvl >= MAXL) return;
    if (p == 0x1000) *v = t->w[lvl]; else if (p == 0x1001) *v = t->h[lvl]; else if (p == 0x1003) *v = 4;
}

// ---- display lists
GLF(GLuint, glGenLists, (GLsizei r)) { GLuint b = list_next; list_next += r; return b; }
GLF(void, glNewList, (GLuint l, GLenum m)) { List *x = list_get(l, 1); x->n = 0; list_comp = l; list_mode = m; }
GLF(void, glEndList, (void)) { list_comp = 0; }
GLF(void, glListBase, (GLuint b)) { S.listbase = b; }
GLF(void, glDeleteLists, (GLuint l, GLsizei r)) {
    for (GLsizei i = 0; i < r; i++) { List *x = list_get(l + i, 0); if (x) { free(x->c); memset(x, 0, sizeof *x); } }
}
static int list_depth;
GLF(void, glCallList, (GLuint id)) {
    REC(OP_CALL, id, 0, 0, 0, 0, 0);
    List *l = list_get(id, 0);
    if (!l || !l->n || list_depth > 32) return;
    GLuint save = list_comp;
    list_comp = 0;
    list_depth++;
    for (int i = 0; i < l->n; i++) {
        const double *a = l->c[i].a;
        switch (l->c[i].op) {
        case OP_BEGIN: glBegin((GLenum)a[0]); break;
        case OP_END: glEnd(); break;
        case OP_V2F: glVertex2f((float)a[0], (float)a[1]); break;
        case OP_V2I: glVertex2i((GLint)a[0], (GLint)a[1]); break;
        case OP_V3F: glVertex3f((float)a[0], (float)a[1], (float)a[2]); break;
        case OP_TC2F: glTexCoord2f((float)a[0], (float)a[1]); break;
        case OP_C4F: glColor4f((float)a[0], (float)a[1], (float)a[2], (float)a[3]); break;
        case OP_C4UB: glColor4ub((GLubyte)a[0], (GLubyte)a[1], (GLubyte)a[2], (GLubyte)a[3]); break;
        case OP_TRANSF: glTranslatef((float)a[0], (float)a[1], (float)a[2]); break;
        case OP_TRANSD: glTranslated(a[0], a[1], a[2]); break;
        case OP_BIND: glBindTexture((GLenum)a[0], (GLuint)a[1]); break;
        case OP_ENABLE: glEnable((GLenum)a[0]); break;
        case OP_DISABLE: glDisable((GLenum)a[0]); break;
        case OP_BLEND: glBlendFunc((GLenum)a[0], (GLenum)a[1]); break;
        case OP_PUSH: glPushMatrix(); break;
        case OP_POP: glPopMatrix(); break;
        case OP_IDENT: glLoadIdentity(); break;
        case OP_MATMODE: glMatrixMode((GLenum)a[0]); break;
        case OP_RECTI: glRecti((GLint)a[0], (GLint)a[1], (GLint)a[2], (GLint)a[3]); break;
        case OP_CALL: glCallList((GLuint)a[0]); break;
        case OP_ALPHA: glAlphaFunc((GLenum)a[0], (float)a[1]); break;
        case OP_DMASK: glDepthMask((GLboolean)a[0]); break;
        case OP_DFUNC: glDepthFunc((GLenum)a[0]); break;
        case OP_TEXENV: glTexEnvi((GLenum)a[0], (GLenum)a[1], (GLint)a[2]); break;
        case OP_TEXPARAM: glTexParameteri((GLenum)a[0], (GLenum)a[1], (GLint)a[2]); break;
        case OP_SHADE: glShadeModel((GLenum)a[0]); break;
        case OP_SCALEF: glScalef((float)a[0], (float)a[1], (float)a[2]); break;
        case OP_ROTF: glRotatef((float)a[0], (float)a[1], (float)a[2], (float)a[3]); break;
        }
    }
    list_depth--;
    list_comp = save;
}

// ---- queries
GLF(const GLubyte *, glGetString, (GLenum n)) {
    switch (n) {
    case 0x1F00: return (const GLubyte *)"ClusterForce";
    case 0x1F01: return (const GLubyte *)"Direct3D 11";
    case 0x1F02: return (const GLubyte *)"1.1.0";
    case 0x1F03: return (const GLubyte *)"GL_ARB_multitexture GL_EXT_texture_env_add WGL_EXT_swap_control ";
    }
    return (const GLubyte *)"";
}
GLF(void, glGetIntegerv, (GLenum p, GLint *v)) {
    switch (p) {
    case 0x0D33: *v = 2048; return;            // MAX_TEXTURE_SIZE (the exe allocates a probe texture of this size)
    case 0x84E2: *v = 2; return;               // MAX_TEXTURE_UNITS_ARB
    case 0x0BA2: v[0] = S.vx; v[1] = S.vy; v[2] = S.vw; v[3] = S.vh; return;
    case 0x0C10: v[0] = S.sx; v[1] = S.sy; v[2] = S.sw; v[3] = S.sh; return;
    case 0x8069: *v = S.bound[S.unit]; return; // TEXTURE_BINDING_2D
    case 0x0D3A: v[0] = 4096; v[1] = 4096; return; // MAX_VIEWPORT_DIMS
    case 0x0D50: case 0x0D52: case 0x0D53: case 0x0D54: *v = 8; return; // SUBPIXEL/RED/GREEN/BLUE_BITS
    case 0x0D55: *v = 0; return;                // ALPHA_BITS: the original framebuffer had none
    case 0x0D56: *v = 24; return;               // DEPTH_BITS
    }
    if (p >= 0x0CF0 && p <= 0x0CF5) { *v = S.unpack[p - 0x0CF0]; return; }
    if (p >= 0x0D00 && p <= 0x0D05) { *v = S.pack[p - 0x0D00]; return; }
    *v = 0;
}
GLF(void, glGetFloatv, (GLenum p, GLfloat *v)) {
    if (p == 0x0BA6) { memcpy(v, mv_st[mv_top].m, 64); return; }
    if (p == 0x0BA7) { memcpy(v, pj_st[pj_top].m, 64); return; }
    GLint i[4] = {0};
    glGetIntegerv(p, i);
    v[0] = (float)i[0];
}

// ---- WGL
static HGLRC g_rc = (HGLRC)(ULONG_PTR)0x1;
GLF(HGLRC, wglCreateContext, (HDC dc)) { return g_rc; }
GLF(BOOL, wglDeleteContext, (HGLRC rc)) { return TRUE; }
GLF(BOOL, wglMakeCurrent, (HDC dc, HGLRC rc)) {
    if (rc && !dev) {
        HWND w = WindowFromDC(dc);
        if (!create_device(w ? w : g_main)) {
            MessageBoxA(NULL, "Direct3D 11 could not be initialised (see d3d.log).", "Clusterball", MB_ICONERROR);
            ExitProcess(1);
        }
    }
    return TRUE;
}
GLF(HGLRC, wglGetCurrentContext, (void)) { return dev ? g_rc : NULL; }
GLF(BOOL, wglSwapIntervalEXT, (int i)) { g_sync = i; return TRUE; }
GLF(int, wglGetSwapIntervalEXT, (void)) { return g_sync; }
GLF(PROC, wglGetProcAddress, (LPCSTR n)) {
    PROC p = NULL;
    if (!strcmp(n, "glActiveTextureARB")) p = (PROC)glActiveTextureARB;
    else if (!strcmp(n, "glClientActiveTextureARB")) p = (PROC)glClientActiveTextureARB;
    else if (!strcmp(n, "glMultiTexCoord2fARB")) p = (PROC)glMultiTexCoord2fARB;
    else if (!strcmp(n, "wglSwapIntervalEXT")) p = (PROC)wglSwapIntervalEXT;
    else if (!strcmp(n, "wglGetSwapIntervalEXT")) p = (PROC)wglGetSwapIntervalEXT;
    DLOG("wglGetProcAddress %s -> %p", n, (void *)p);
    return p;
}

// Writes a texture (BGRA8 or R8) as 24-bit BMP named <base><suffix>.bmp.
static int g_script_shots;
static void dump_texture(ID3D11Texture2D *t, const char *base, const char *suffix) {
    D3D11_TEXTURE2D_DESC d;
    ID3D11Texture2D_GetDesc(t, &d);
    d.Usage = D3D11_USAGE_STAGING; d.BindFlags = 0; d.CPUAccessFlags = D3D11_CPU_ACCESS_READ; d.MiscFlags = 0;
    ID3D11Texture2D *st = NULL;
    if (FAILED(ID3D11Device_CreateTexture2D(dev, &d, NULL, &st))) return;
    ID3D11DeviceContext_CopyResource(ctx, (ID3D11Resource *)st, (ID3D11Resource *)t);
    D3D11_MAPPED_SUBRESOURCE m;
    if (SUCCEEDED(ID3D11DeviceContext_Map(ctx, (ID3D11Resource *)st, 0, D3D11_MAP_READ, 0, &m))) {
        char name[128];
        sprintf(name, "%s%s.bmp", base, suffix);
        if (d.Format == DXGI_FORMAT_R8_UNORM) { // expand grey to BGRA
            BYTE *px = malloc((size_t)d.Width * d.Height * 4);
            for (UINT y = 0; y < d.Height; y++)
                for (UINT x = 0; x < d.Width; x++) {
                    BYTE g = ((BYTE *)m.pData)[y * m.RowPitch + x];
                    BYTE *o = px + ((size_t)y * d.Width + x) * 4;
                    o[0] = o[1] = o[2] = g; o[3] = 255;
                }
            write_bmp24_topdown(name, d.Width, d.Height, px, d.Width * 4);
            free(px);
        } else write_bmp24_topdown(name, d.Width, d.Height, m.pData, m.RowPitch);
        ID3D11DeviceContext_Unmap(ctx, (ID3D11Resource *)st, 0);
    }
    ID3D11Texture2D_Release(st);
}

// Run-time settings for the test script ("set <name> <value>").
void backend_set(const char *k, double v) {
    if (!strcmp(k, "sharpen")) g_sharpen = (float)v;
    else if (!strcmp(k, "contrast")) g_contrast = (float)v;
    else if (!strcmp(k, "saturation")) g_saturation = (float)v;
    else if (!strcmp(k, "gamma")) g_gamma = (float)v;
    else if (!strcmp(k, "ao")) g_ao_on = v != 0;
    else if (!strcmp(k, "aostrength")) g_ao_strength = (float)v;
    else if (!strcmp(k, "aoradius")) g_ao_radius = (float)v;
    else if (!strcmp(k, "texlog")) g_texlog = (int)v;
    else if (!strcmp(k, "sharpfont")) g_sharp_font = v != 0;
    else if (!strcmp(k, "fontreplace")) g_font_replace = v != 0;
    else if (!strcmp(k, "uvclamp")) g_uv_clamp = v != 0;
    else if (!strcmp(k, "texreplace")) { // re-create every texture with or without its replacement
        g_tex_replace = v != 0;
        for (int i = 0; i < 1024; i++)
            for (Tex *t = tex_tab[i]; t; t = t->next) t->dirty = 1;
    }
    DLOG("set %s = %g", k, v);
}

// ---------------------------------------------------------------- present (called from the SwapBuffers hook)
BOOL backend_swap(HDC dc, int sec) {
    if (!dev) return TRUE;
    motion_tick();
    ID3D11Texture2D *bb = NULL;
    if (FAILED(IDXGISwapChain_GetBuffer(sc, 0, &IID_ID3D11Texture2D, (void **)&bb))) return TRUE;
    // 1. window-sized source image in post_tex
    if (g_ss == 1) {
        if (g_msaa > 1) ID3D11DeviceContext_ResolveSubresource(ctx, (ID3D11Resource *)post_tex, 0, (ID3D11Resource *)rt_tex, 0, DXGI_FORMAT_B8G8R8A8_UNORM);
        else ID3D11DeviceContext_CopyResource(ctx, (ID3D11Resource *)post_tex, (ID3D11Resource *)rt_tex);
    } else {
        if (res_tex) ID3D11DeviceContext_ResolveSubresource(ctx, (ID3D11Resource *)res_tex, 0, (ID3D11Resource *)rt_tex, 0, DXGI_FORMAT_B8G8R8A8_UNORM);
        // box filter: every output pixel averages its ss x ss block of the supersampled image
        ID3D11DeviceContext_PSSetShaderResources(ctx, 0, 1, &src_srv);
        fullscreen_pass(post_rtv, out_w, out_h, down_ps, get_blend(0, 1, 0, 1));
        unbind_srvs();
        // post_tex is read next; while it is still bound as render target, Direct3D silently binds a null
        // shader resource instead and the image comes out black.
        ID3D11DeviceContext_OMSetRenderTargets(ctx, 0, NULL, NULL);
    }
    // 2. sharpening and colour grading into the back buffer
    write_pbuf();
    ID3D11RenderTargetView *bbv = NULL;
    ID3D11Device_CreateRenderTargetView(dev, (ID3D11Resource *)bb, NULL, &bbv);
    ID3D11DeviceContext_PSSetShaderResources(ctx, 0, 1, &post_srv);
    fullscreen_pass(bbv, out_w, out_h, post_ps, get_blend(0, 1, 0, 1));
    unbind_srvs();
    ID3D11DeviceContext_OMSetRenderTargets(ctx, 1, &rtv, dsv);
    ID3D11RenderTargetView_Release(bbv);
    if (sec >= 0) {
        extern char g_shot_name[64];
        char base[96];
        CreateDirectoryA("shots", NULL); // in the game directory
        if (g_shot_name[0]) sprintf(base, "shots\\shot_%s", g_shot_name);
        else sprintf(base, "shots\\frame_%02d", sec);
        g_shot_name[0] = 0;
        dump_texture(bb, base, "");             // what is shown
        if (g_script_shots) {
            dump_texture(post_tex, base, "_raw"); // same frame before sharpening and colour grading
            dump_texture(ao_tex, base, "_ao");    // ambient-occlusion mask (unblurred)
        }
    }
    ID3D11Texture2D_Release(bb);
    HRESULT hr = IDXGISwapChain_Present(sc, (g_sync > 0 && g_vsync_cfg) ? 1 : 0, 0);
    if (FAILED(hr)) DLOG("Present failed %08lx", hr);
    return TRUE;
}

// ---------------------------------------------------------------- pass-through table for the remaining exports
// GLU32.dll binds to this module by name; GL entry points the game never uses go to the system GL (no context: no-op).
extern void *g_pass[];
extern const char *g_pass_names[];

BOOL WINAPI DllMain(HINSTANCE h, DWORD why, LPVOID r) {
    if (why == DLL_PROCESS_ATTACH) {
        state_defaults();
        char p[MAX_PATH];
        GetSystemDirectoryA(p, MAX_PATH);
        strcat(p, "\\opengl32.dll");
        HMODULE real = LoadLibraryA(p);
        for (int i = 0; g_pass_names[i]; i++) g_pass[i] = real ? (void *)GetProcAddress(real, g_pass_names[i]) : NULL;
        extern double cfg_num(const char *env, const char *key, double def);
        g_aniso = (int)cfg_num("CB_ANISO", "Anisotropy", 8);
        g_vsync_cfg = cfg_num("CB_VSYNC", "VSync", 1) != 0;
        g_latency = (int)cfg_num("CB_LATENCY", "MaxFrameLatency", 1);
        g_msaa = (int)cfg_num("CB_MSAA", "MSAA", 4);
        g_ss = (int)cfg_num("CB_SUPERSAMPLE", "SuperSample", 1);
        if (g_ss < 1) g_ss = 1;
        if (g_ss > 4) g_ss = 4;
        g_sharpen = (float)cfg_num("CB_SHARPEN", "Sharpen", 0.5);
        g_contrast = (float)cfg_num("CB_CONTRAST", "Contrast", 1.05);
        g_saturation = (float)cfg_num("CB_SATURATION", "Saturation", 1.1);
        g_gamma = (float)cfg_num("CB_GAMMA", "Gamma", 1.0);
        if (g_gamma <= 0.1f) g_gamma = 1.0f;
        g_ao_on = cfg_num("CB_AO", "AmbientOcclusion", 0) != 0; // off: the coarse terrain shows its facet edges
        g_sharp_font = cfg_num("CB_SHARPFONT", "SharpFont", 1) != 0;
        g_font_replace = cfg_num("CB_FONTREPLACE", "FontReplace", 1) != 0;
        g_uv_clamp = cfg_num("CB_UVCLAMP", "EdgeClamp2D", 1) != 0;
        g_texlog = (int)cfg_num("CB_TEXLOG", "TexLog", 0); // diagnostics: log the first N 2D draws
        g_log_unrep = cfg_num("CB_LOGUNREPLACED", "LogUnreplaced", 0) != 0; // diagnostics: 2D textures without a pack file
        g_tex_dump = cfg_num("CB_TEXDUMP", "TextureDump", 0) != 0;
        g_tex_replace = cfg_num("CB_TEXREPLACE", "TextureReplace", 1) != 0;
        DLOG("settings: msaa %d, supersample %d, sharpen %.2f, ao %d, texture replace %d", g_msaa, g_ss, g_sharpen, g_ao_on, g_tex_replace);
        {
            extern int cfg_str(const char *env, const char *key, char *out, int n);
            char pk[64];
            if (cfg_str("CB_TEXPACK", "TexturePack", pk, sizeof pk) && !strchr(pk, '\\') && !strchr(pk, '/')) strcpy(g_tex_pack, pk);
        }
        {
            GetModuleFileNameA(NULL, g_tex_dir, MAX_PATH);
            char *s = strrchr(g_tex_dir, '\\');
            strcpy(s ? s + 1 : g_tex_dir, "textures");
            if (g_tex_dump) {
                char d[MAX_PATH];
                CreateDirectoryA(g_tex_dir, NULL);
                sprintf(d, "%s\\dump", g_tex_dir);
                CreateDirectoryA(d, NULL);
            }
        }
        g_ao_strength = (float)cfg_num("CB_AOSTRENGTH", "AOStrength", 0.6);
        g_ao_radius = (float)cfg_num("CB_AORADIUS", "AORadius", 200);
        { char sp[8]; g_script_shots = GetEnvironmentVariableA("CB_SCRIPT", sp, sizeof sp) > 0; }
        if (g_latency < 1) g_latency = 1;
        compat_init();
    }
    return TRUE;
}
