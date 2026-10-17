package core

// PlayerIconBase is where the player logos in docs/players are served from. Discord fetches the small icon of a
// status by its address (through its own image proxy), so nothing is uploaded and nothing about the user is sent.
const PlayerIconBase = "https://raw.githubusercontent.com/CptPakundo/MPCvibedRPC/master/docs/players/"

// playerIcons names the logo file of each player that has one (the MPC family uses the Discord application's own
// mpc-hc image).
var playerIcons = map[string]string{
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
