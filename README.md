# Motion Detection Security Camera System

A Python-based motion detection system that monitors a webcam and sends alerts via Telegram. Features automated motion-triggered recording and manual recording via Telegram commands.

## Features

- **Automated Motion Detection**: Detects motion and captures snapshots
- **Video Recording**: Records 15-second videos after sustained motion (5+ seconds)
- **Telegram Integration**: 
  - Receive instant motion alerts with snapshots
  - Send commands to start/stop manual recording
  - Retrieve recorded videos on demand
- **Day/Night Mode**: Adjustable sensitivity for different lighting conditions
- **Manual Recording**: High-quality 15 FPS recording on demand via Telegram
- **Efficient Resource Usage**: 8 FPS for automated recordings to save resources
- **Heartbeat Monitoring**: Daily status updates

## Requirements

- Python 3.7+
- Webcam
- Telegram Bot Token and Chat ID

## Installation

1. Clone this repository:
```bash
git clone https://github.com/yourusername/motion-detector.git
cd motion-detector
```

2. Install dependencies:
```bash
pip install -r requirements.txt
```

3. Configure Telegram credentials:
```bash
# Copy the example file
copy .env.example .env

# Edit .env file and add your credentials:
# TELEGRAM_BOT_TOKEN=your_bot_token_here
# TELEGRAM_CHAT_ID=your_chat_id_here
```

4. Run the application:
```bash
python motion_detector.py
```

## Usage

### Running the System

```bash
python motion_detector.py
```

On Windows, you can also use the provided batch file:
```bash
start_motion.cmd
```

### Telegram Commands

- `/record` or `record` - Start manual recording
- `/stop` or `stop` - Stop manual recording and receive the video
- `/get` or `get` - List all manual recordings and select one to download

## Configuration

Edit these variables in `motion_detector.py` to customize:

```python
# Camera
CAMERA_INDEX = 0  # Change if using external webcam

# Motion Detection Timing
MOTION_PERSIST_SECONDS = 5   # Seconds of motion before video recording starts
VIDEO_DURATION_SECONDS = 15  # Length of automatic recordings

# Alert Cooldowns (seconds between snapshots)
COOLDOWN_DAY = 15
COOLDOWN_NIGHT = 30

# Day Detection Thresholds
DAY_MIN_CONTOUR_AREA = 8000
DAY_THRESH_VALUE = 30
DAY_FRAME_INTERVAL = 1  # Check every 1 second

# Night Detection Thresholds
NIGHT_MIN_CONTOUR_AREA = 12000
NIGHT_THRESH_VALUE = 40
NIGHT_FRAME_INTERVAL = 2  # Check every 2 seconds

# Day/Night Hours
NIGHT_START = 20  # 8 PM
NIGHT_END = 6     # 6 AM
```

## File Structure

```
motion/
├── motion_detector.py    # Main application
├── start_motion.cmd      # Windows launcher
├── snapshots/           # Motion snapshots (created automatically)
├── videos/              # Recorded videos (created automatically)
├── DISABLED             # Create this file to disable alerts
└── README.md            # This file
```

## How It Works

1. **Motion Detection**: Continuously monitors camera feed for motion using OpenCV
2. **Snapshot Alerts**: When motion detected, saves snapshot and sends to Telegram
3. **Persistent Motion**: If motion continues for 5+ seconds, starts recording video
4. **Automatic Recording**: Records for 15 seconds at 8 FPS, sends to Telegram
5. **Manual Recording**: On-demand recording at 15 FPS until stopped

## Disabling Alerts

To temporarily disable Telegram alerts (but keep recording):
```bash
# Create DISABLED file
echo. > DISABLED

# To re-enable, delete the file
del DISABLED
```

## Video Quality

- **Manual Recordings**: 15 FPS, full quality, runs until you stop it
- **Automatic Recordings**: 8 FPS, efficient quality, 15 seconds duration

## Troubleshooting

### Camera not working
- Check `CAMERA_INDEX` value (try 0, 1, 2)
- Ensure camera permissions are granted
- Close other applications using the camera

### Telegram not working
- Verify `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` are correct
- Test by sending a message directly to your bot
- Check internet connection

### Videos play too fast
- This is tuned for typical webcams with processing overhead
- If still fast, reduce FPS values in the code

## Security Notes

⚠️ **Important**: Never commit your Telegram credentials to GitHub!
- Use environment variables for tokens
- Add credentials to `.gitignore` if stored in a config file

## License

MIT License - Feel free to use and modify for your needs.

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## Acknowledgments

- Built with OpenCV for motion detection
- Telegram Bot API for notifications
- Inspired by home security needs
