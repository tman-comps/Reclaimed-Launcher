# Steam library art

Source for `internal/steam/art/`, the images Add to Steam puts in Steam's
grid folder. `art.html` lays out the built-in covers
(`internal/art/defaults/*`) and `build/appicon.png`. Copy those next to it,
then render each kind with headless Edge at Steam's sizes:

| `?k=` | Size | Saved as |
|---|---|---|
| `portrait` | 600×900 | `portrait.jpg` (library cover) |
| `wide` | 920×430 | `wide.jpg` (recent games / banner) |
| `hero` | 1920×620 | `hero.jpg` (top of the game's page) |
| `logo` | 1280×320, transparent | `logo.png` (drawn over the hero) |

```powershell
msedge --headless=new --hide-scrollbars --force-device-scale-factor=1 --allow-file-access-from-files --default-background-color=00000000 --window-size=600,900 --screenshot=portrait.png "file:///<folder>/art.html?k=portrait"
```

The photos are saved as JPG (quality 88) to keep the exe small.
