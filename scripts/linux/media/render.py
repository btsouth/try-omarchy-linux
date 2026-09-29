#!/usr/bin/env python3
"""Render the Linux presentation from the official vector mark and a real capture.
Requires rsvg-convert and ffmpeg. Run on devbox, not the user's desktop.
"""
from pathlib import Path
import math, subprocess, tempfile, xml.etree.ElementTree as ET
import argparse
p=argparse.ArgumentParser();p.add_argument('capture',type=Path);p.add_argument('out',type=Path);a=p.parse_args()
a.out.mkdir(parents=True,exist_ok=True)
root=ET.parse(Path(__file__).with_name('omarchy-wordmark.svg')).getroot()
cells=[]
for rect in root.findall('.//{http://www.w3.org/2000/svg}rect'):
 x,y,w,h=[int(rect.attrib[k]) for k in ('x','y','width','height')]
 cells += [(xx,yy) for yy in range(y//50,(y+h)//50) for xx in range(x//51,(x+w)//51)]
def svg(t):
 s=['<svg xmlns="http://www.w3.org/2000/svg" width="1280" height="720" viewBox="0 0 1280 720">', '<rect width="1280" height="720" fill="#11150f"/>']
 # Restrained pixel field around the original, unmodified glyph.
 for y in range(16,705,16):
  for x in range(16,1265,16):
   n=((x//16)*73+(y//16)*151)%97
   if n<12:
    v=.12+.09*math.sin(x*.011+y*.013-t*.7)
    s.append(f'<rect x="{x}" y="{y}" width="2" height="2" fill="#9ece6a" opacity="{v:.3f}"/>')
 s += ['<path d="M56 90V56H90 M1190 56H1224V90 M56 630V664H90 M1190 664H1224V630" fill="none" stroke="#39482e" stroke-width="2"/>',
 '<text x="640" y="184" text-anchor="middle" font-family="DejaVu Sans Mono,monospace" font-size="26" letter-spacing="10" fill="#c7d4bd">TRY</text>']
 for x,y in cells:
  crest=math.exp(-((x-(t*30-12))/8)**2)*.65 if t>.45 else 0
  color='#d8edbf' if crest>.4 else '#9ece6a'
  s.append(f'<rect x="{154+x*12}" y="{246+y*12}" width="12" height="12" fill="{color}"/>')
 s += ['<text x="640" y="552" text-anchor="middle" font-family="DejaVu Sans Mono,monospace" font-size="23" letter-spacing="5" fill="#9ece6a">FOR LINUX</text>', '<text x="640" y="605" text-anchor="middle" font-family="DejaVu Sans,sans-serif" font-size="25" fill="#c7d4bd">The full Omarchy desktop. In a window.</text>','</svg>']
 return ''.join(s)
def run(*cmd): subprocess.run(cmd,check=True)
with tempfile.TemporaryDirectory(prefix='try-omarchy-media-') as td:
 d=Path(td)
 for i in range(48):
  f=d/f'{i:03}.svg';f.write_text(svg(i/12));run('rsvg-convert',str(f),'-o',str(d/f'{i:03}.png'))
 (a.out/'try-omarchy-linux-hero.svg').write_text(svg(0))
 run('rsvg-convert',str(a.out/'try-omarchy-linux-hero.svg'),'-o',str(a.out/'try-omarchy-linux-hero.png'))
 common=['ffmpeg','-hide_banner','-loglevel','error','-y']
 run(*common,'-framerate','12','-i',str(d/'%03d.png'),'-loop','1','-framerate','12','-i',str(a.capture),'-loop','1','-framerate','12','-i',str(a.out/'try-omarchy-linux-hero.png'),'-filter_complex','[0:v]format=yuv420p,settb=AVTB[a];[1:v]scale=1280:720:flags=lanczos,format=yuv420p,settb=AVTB[b];[2:v]format=yuv420p,settb=AVTB[c];[a][b]xfade=transition=fade:duration=0.65:offset=3.3[ab];[ab][c]xfade=transition=fade:duration=0.65:offset=8.3[v]','-map','[v]','-t','10','-an','-c:v','libvpx-vp9','-crf','30','-b:v','0','-threads','4',str(a.out/'try-omarchy-linux.webm'))
 run(*common,'-i',str(a.out/'try-omarchy-linux.webm'),'-filter_complex','fps=12,scale=960:540:flags=lanczos,split[a][b];[a]palettegen=stats_mode=diff[p];[b][p]paletteuse=dither=bayer:bayer_scale=3','-loop','0',str(a.out/'try-omarchy-linux.gif'))
 run(*common,'-i',str(a.capture),'-frames:v','1','-q:v','2',str(a.out/'try-omarchy-linux-desktop.jpg'))
