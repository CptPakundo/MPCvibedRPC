package core

// PlayerIconBase is where the player logos in docs/players are served from. Discord fetches the small icon of a
// status by its address (through its own image proxy), so nothing is uploaded and nothing about the user is sent.
const PlayerIconBase = "https://raw.githubusercontent.com/CptPakundo/MPCvibedRPC/master/docs/players/"

// playerIcons names the logo file of each player that has one. The MPC family has its own too: the default Discord
// application has no mpc-hc image (its assets are default, pause_small, play_small, stop_small and mpcbe_logo, checked
// 2026-11-21), so the key "mpc-hc" showed no image at all.
var playerIcons = map[string]string{
	"MPC-HC": "mpc-hc", "MPC-BE": "mpc-be", "MPC-QT": "mpc-qt",
	"mpv": "mpv", "VLC": "vlc", "IINA": "iina", "PotPlayer": "potplayer", "Kodi": "kodi", "SMPlayer": "smplayer",
	"Plex": "plex", "Stremio": "stremio", "Jellyfin": "jellyfin", "Emby": "emby",
}

// PlayerIcon is the address of the player's logo for the small icon of a status ("" when there is none).
func PlayerIcon(player string) string {
	if n, ok := playerIcons[player]; ok {
		return PlayerIconBase + n + ".png"
	}
	return ""
}
