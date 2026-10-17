## MPCvibedRPC 0.9.33

**TVmaze covers work again, and every address the program connects to is now listed in one place.**

### What's new
- **TVmaze:** its search now takes 30 to 60 seconds for a show it has not been asked about lately (then it answers at
  once for an hour). The program gave up after a few seconds, so shows found only by TVmaze got no cover and no episode
  names. Now it carries on without TVmaze, keeps waiting in the background, and looks the title up again as soon as the
  answer is in: the cover appears about a minute after the show starts, the first time.
- **Where it connects:** [SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md#where-it-connects)
  lists every address the program asks, every image the window and Discord load, and every page it only links to. The
  Privacy tab links to it, and a test fails whenever the code names an address that is not on the list.
- **MPC-HC's web interface:** the Privacy tab now says that it has no password, so web pages you open can read it and
  control the player while it is on. Switch it off in MPC-HC (Options > Player > Web Interface) when you stop using
  this program.

### Get it
- **Windows:** download **`MPCvibedRPC.exe`** and run it, or install it from the Updates section of the version you
  have. Your settings are kept.
- **macOS 13 or later:** download **`MPCvibedRPC-macos.zip`** and move the app to Applications (the first time,
  System Settings > Privacy & Security > **Open Anyway**).
- **Linux:** download **`MPCvibedRPC-linux-amd64`** (or `-arm64`), `chmod +x` it and run it.

Every system needs the Discord desktop app with User Settings > Activity Privacy > "Share my activity" on (Activity
Privacy is in the Games & Apps section, called Activity Settings in older Discord versions). To check a download, see
[SECURITY.md](https://github.com/CptPakundo/MPCvibedRPC/blob/master/SECURITY.md).
