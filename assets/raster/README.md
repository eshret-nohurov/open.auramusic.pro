# Растровые знаки логотипа для карточки

Go не рисует SVG, поэтому знак переведён в PNG 256×256 с прозрачным фоном через headless Chrome (librsvg не ставится).
Viewbox подрезан по краю знака, чтобы он занимал весь квадрат. Пересоздать из корня репо:

    T=$(mktemp -d); for n in logo-mark logo-mark-mono; do sed 's/width="52" height="51" viewBox="0 0 52 51"/width="256" height="256" viewBox="0 0 51.3689 51" preserveAspectRatio="none"/' assets/static/brand/$n.svg > $T/$n.svg; printf '<!doctype html><html><body style="margin:0;background:transparent"><img src="%s.svg" style="display:block"></body></html>' $n > $T/$n.html; "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome" --headless --disable-gpu --hide-scrollbars --default-background-color=00000000 --window-size=256,256 --screenshot=$PWD/assets/raster/$n-256.png file://$T/$n.html; done
