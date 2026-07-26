# 从新 logo 源图生成项目全套图标
# 用法: python build/crop_logo.py
from PIL import Image
import numpy as np
import shutil

SRC = r"D:\0_My\我\1.png"
ROOT = r"d:\1_Study\Project\Active\LinkStar\learn\linkstar"

src = Image.open(SRC).convert("RGBA")
a = np.array(src)
h, w = a.shape[:2]
print("原始尺寸:", (w, h))

rgb = a[..., :3].astype(np.int16)
white = (rgb >= 235).all(axis=-1)  # 近白像素

# 从四边泛洪，找出与外部连通的白色背景
reach = np.zeros_like(white)
reach[0, :] = white[0, :]
reach[-1, :] = white[-1, :]
reach[:, 0] = white[:, 0]
reach[:, -1] = white[:, -1]
while True:
    grow = reach.copy()
    grow[1:, :] |= reach[:-1, :]
    grow[:-1, :] |= reach[1:, :]
    grow[:, 1:] |= reach[:, :-1]
    grow[:, :-1] |= reach[:, 1:]
    grow &= white
    if (grow == reach).all():
        break
    reach = grow

# 背景区向内扩 3px 覆盖抗锯齿过渡带
band = reach.copy()
for _ in range(3):
    g = band.copy()
    g[1:, :] |= band[:-1, :]
    g[:-1, :] |= band[1:, :]
    g[:, 1:] |= band[:, :-1]
    g[:, :-1] |= band[:, 1:]
    band = g

# alpha：背景及过渡带按“离白距离”渐变，纯白=0；logo 主体不透明
dist = (255 - rgb.min(axis=-1)).astype(np.float32)
soft = np.clip(dist * 1.2, 0, 255).astype(np.uint8)
alpha = np.full((h, w), 255, np.uint8)
alpha[band] = soft[band]
alpha[reach] = np.minimum(alpha[reach], soft[reach])

out = a.copy()
out[..., 3] = alpha

# 按内容裁切，留 4% 边距，补成正方形
ys, xs = np.where(~reach)
top, bottom, left, right = ys.min(), ys.max(), xs.min(), xs.max()
cw, ch = right - left, bottom - top
pad = int(max(cw, ch) * 0.04)
side = max(cw, ch) + 2 * pad
cx, cy = (left + right) // 2, (top + bottom) // 2
x0, y0 = max(0, cx - side // 2), max(0, cy - side // 2)
x1, y1 = min(w, x0 + side), min(h, y0 + side)
base = Image.fromarray(out, "RGBA").crop((x0, y0, x1, y1))
print("裁切后:", base.size)

def emit(path, size):
    img = base.resize((size, size), Image.LANCZOS)
    img.save(ROOT + path)
    print("生成:", path, f"{size}x{size}")

emit(r"\docs\img\logo.png", 512)              # README 页首
emit(r"\icon_256.png", 256)                   # 备用
emit(r"\icon_64.png", 64)                     # 桌面版托盘图标（embed）
emit(r"\build\appicon.png", 1024)             # Wails 应用图标
emit(r"\web\admin\public\favicon.png", 128)   # 管理后台 favicon
emit(r"\web\home\public\favicon.png", 128)    # 首页 favicon
emit(r"\web\admin\src\assets\logo.png", 128)  # 管理后台侧边栏 logo

# Windows 多尺寸 ico（Wails 桌面版 exe 图标）
base.resize((256, 256), Image.LANCZOS).save(
    ROOT + r"\build\windows\icon.ico",
    sizes=[(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)],
)
print("生成: \\build\\windows\\icon.ico (multi-size)")

# 原始源图归档，替换旧的 lICON.PNG
shutil.copyfile(SRC, ROOT + r"\lICON.PNG")
print("已更新 lICON.PNG（新源图副本）")
