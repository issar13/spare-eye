import {format, parseISO} from 'date-fns';
import {Arm, Disarm, LogLines, OpenExternally, OpenRecordingsFolder, StartManualRecording, StopManualRecording, TakeSnapshot} from '../wailsjs/go/main/App';
import type {app} from '../wailsjs/go/models';
import {hideBanner, showBanner, showResult} from './banner';
import {clearButtonPending, setButtonPending} from './button-state';
import {requireElement} from './elements';
import {formatElapsedSeconds, formatWhen} from './format';
import {guideText} from './home-guide';
import {loadRecentEvents, loadRecordings} from './queries';

const heroDot = requireElement<HTMLElement>('#hero-dot');
const heroHeadline = requireElement<HTMLElement>('#hero-headline');
const heroExplanation = requireElement<HTMLElement>('#hero-explanation');
const heroGuide = requireElement<HTMLElement>('#hero-guide');
const heroStateChip = requireElement<HTMLElement>('#hero-state-chip');
const heroStateLabel = requireElement<HTMLElement>('#hero-state-label');
const heroProfileChip = requireElement<HTMLElement>('#hero-profile-chip');
const heroLocationChip = requireElement<HTMLElement>('#hero-location-chip');
const messageBanner = requireElement<HTMLElement>('#home-message');
const watchButton = requireElement<HTMLButtonElement>('#watch-button');
const recordButton = requireElement<HTMLButtonElement>('#record-button');
const snapshotButton = requireElement<HTMLButtonElement>('#snapshot-button');
const diagnostics = requireElement<HTMLElement>('#diagnostics');
const healthTag = requireElement<HTMLElement>('#health-tag');
const feedFps = requireElement<HTMLElement>('#feed-fps');
const feedSize = requireElement<HTMLElement>('#feed-size');
const mediaDirectory = requireElement<HTMLElement>('#media-directory');
const alertsChip = requireElement<HTMLElement>('#alerts-chip');
const eventList = requireElement<HTMLElement>('#event-list');
const logView = requireElement<HTMLPreElement>('#log-view');
const activityMessage = requireElement<HTMLElement>('#activity-message');
const recentClips = requireElement<HTMLElement>('#recent-clips');
const recentCount = requireElement<HTMLElement>('#recent-count');
const viewport = requireElement<HTMLElement>('#viewport');

const recentClipLimit = 4;

// busy suppresses the poll's button updates while starting or stopping is in
// flight. Opening a camera takes a second or two, and letting the poll reset
// the label mid-flight would make the button flicker.
let busy = false;
let recordingBusy = false;
let snapshotBusy = false;

// The clip names last drawn, so the two-second refresh only rebuilds the cards
// (and reloads their videos) when a clip has actually come or gone.
let shownClipNames = '';

export const updateHome = (status: app.StatusView): void => {
    heroDot.className = `hero-dot ${status.tone}`;
    heroHeadline.textContent = status.headline;
    heroExplanation.textContent = status.explanation;
    heroStateChip.className = status.armed ? 'tag tag-green' : 'tag';
    heroStateLabel.textContent = status.armed ? 'ACTIVE' : 'OFF';
    heroProfileChip.textContent = status.night ? 'Night profile • Offline local mode' : 'Daylight profile • Offline local mode';
    heroLocationChip.hidden = status.cameraLocation === '';
    heroLocationChip.textContent = status.cameraLocation;

    if (!busy) {
        watchButton.textContent = status.armed ? 'Stop watching' : 'Start watching';
        watchButton.disabled = !status.armed && !status.canArm;
    }

    // Whichever action is the useful next one gets the solid button: starting
    // the camera while it is off, recording once it is on.
    watchButton.classList.toggle('primary', !status.armed);
    watchButton.classList.toggle('i-pause', status.armed);
    watchButton.classList.toggle('i-play', !status.armed);
    recordButton.classList.toggle('primary', status.armed);

    if (!recordingBusy) {
        recordButton.disabled = !status.armed;
        recordButton.textContent = status.recordingManual ? 'Stop recording' : 'Record now';
    }
    if (!snapshotBusy) {
        snapshotButton.disabled = !status.armed;
    }
    recordButton.classList.toggle('i-record', !status.recordingManual);
    recordButton.classList.toggle('i-stop', status.recordingManual);

    feedFps.textContent = status.captureFPS === 0 ? 'Not measured yet' : `Capturing • ${status.captureFPS.toFixed(1)} fps`;
    feedSize.textContent = status.frameWidth === 0 ? 'Picture size unknown' : `${status.frameWidth} × ${status.frameHeight}`;

    mediaDirectory.textContent = status.mediaDirectory;

    renderGuide(status.problem);
    renderDiagnostics(status);
    renderAlerts(status);
};

// renderGuide points at the written walkthrough when one would help. The
// obstacle itself is already the hero headline, and its remedy the explanation
// underneath, so only the pointer to the guide is left to add.
const renderGuide = (problem: app.Problem | null | undefined): void => {
    const text = guideText(problem);

    heroGuide.hidden = text === '';
    heroGuide.textContent = text;
};

// renderDiagnostics fills the Camera Health card with numbers measured by the
// detector. Anything not measured yet says so rather than showing a guess.
const renderDiagnostics = (status: app.StatusView): void => {
    healthTag.textContent = status.armed ? (status.frameWidth === 0 ? 'Connecting…' : 'Camera live') : 'Standing by';
    healthTag.className = status.armed ? 'side-tag tint-green' : 'side-tag muted';

    const entries: [string, string][] = [
        ['Camera', status.cameraSource],
        ['Picture size', status.frameWidth === 0 ? 'Not measured yet' : `${status.frameWidth} × ${status.frameHeight}`],
        ['Frames per second', status.captureFPS === 0 ? 'Not measured yet' : status.captureFPS.toFixed(1)],
        ['Lighting profile', status.night ? 'Night' : 'Daylight'],
        ['Moving for', status.motionSeconds === 0 ? 'Nothing moving' : formatElapsedSeconds(status.motionSeconds)],
        ['Log file', status.logPath],
    ];

    if (status.lastFailure !== '') {
        entries.push(['Last engine error', status.lastFailure]);
    }

    diagnostics.replaceChildren(...entries.map(([term, value]) => {
        const row = document.createElement('div');
        row.className = 'kv-row';

        const label = document.createElement('span');
        label.textContent = term;

        const detail = document.createElement('span');
        detail.textContent = value;

        row.append(label, detail);
        return row;
    }));
};

const renderAlerts = (status: app.StatusView): void => {
    if (!status.alertsEnabled) {
        alertsChip.className = 'tag';
        alertsChip.textContent = 'Turned off';
        return;
    }
    if (!status.alertsConfigured) {
        // Telegram is optional, so leaving it unset up is not a fault.
        alertsChip.className = 'tag';
        alertsChip.textContent = 'Not set up (optional)';
        return;
    }

    alertsChip.className = status.telegramConnected ? 'tag good' : 'tag warn';
    alertsChip.textContent = status.telegramConnected ? 'Connected' : 'Connecting…';
};

export const refreshActivity = async (): Promise<void> => {
    try {
        const [events, lines] = await Promise.all([loadRecentEvents(), LogLines()]);

        renderEvents(events);
        logView.textContent = lines.join('\n');
        hideBanner(activityMessage);
    } catch {
        const failed = document.createElement('p');
        failed.className = 'hint';
        failed.textContent = 'Recent activity is unavailable right now.';
        eventList.replaceChildren(failed);
        logView.textContent = '';
        showBanner(activityMessage, 'SpareEye could not load recent activity. It will try again shortly.', 'warn');
    }
};

const renderEvents = (events: app.EventView[]): void => {
    if (events.length === 0) {
        const empty = document.createElement('p');
        empty.className = 'hint';
        empty.textContent = 'Nothing has happened yet. Once you start watching, activity shows up here.';
        eventList.replaceChildren(empty);
        return;
    }

    eventList.replaceChildren(...events.map((event) => {
        const row = document.createElement('div');
        row.className = `event ${event.kind}`;

        const when = document.createElement('time');
        when.textContent = formatWhen(String(event.at));

        const message = document.createElement('span');
        message.className = 'event-message';
        message.textContent = event.message;

        const name = document.createElement('span');
        name.className = 'event-file';
        name.textContent = event.mediaName;

        row.append(when, message, name);
        return row;
    }));
};

export const refreshRecentClips = async (): Promise<void> => {
    try {
        const clips = (await loadRecordings('clips')).slice(0, recentClipLimit);
        const names = clips.map((clip) => clip.name).join('\n');

        if (names === shownClipNames && recentClips.childElementCount > 0) {
            return;
        }
        shownClipNames = names;
        recentCount.textContent = clips.length === 1 ? '1 clip' : `${clips.length} clips`;

        if (clips.length === 0) {
            const empty = document.createElement('p');
            empty.className = 'hint';
            empty.textContent = 'No motion clips yet. Once something moves in view, the latest clips appear here.';
            recentClips.replaceChildren(empty);
            return;
        }

        recentClips.replaceChildren(...clips.map(renderClipCard));
    } catch {
        shownClipNames = '';
        const failed = document.createElement('p');
        failed.className = 'hint';
        failed.textContent = 'Recent clips could not be loaded. Your recordings folder may be unavailable.';
        recentClips.replaceChildren(failed);
    }
};

const renderClipCard = (clip: app.RecordingView): HTMLElement => {
    const card = document.createElement('article');
    card.className = 'clip-card';

    const thumb = document.createElement('div');
    thumb.className = 'clip-thumb';

    // The poster is a JPEG saved with the clip, shown until it is played.
    // Without a poster, or if it fails to load, the dark tile behind it shows.
    const poster = document.createElement('img');
    poster.alt = `Clip from ${format(parseISO(clip.createdAt), 'HH:mm:ss')}`;
    poster.loading = 'lazy';
    poster.addEventListener('error', () => poster.remove());
    if (clip.posterURL) {
        poster.src = clip.posterURL;
    } else {
        poster.hidden = true;
    }

    const time = document.createElement('span');
    time.className = 'glass-pill mono';
    time.textContent = format(parseISO(clip.createdAt), 'HH:mm:ss');

    thumb.append(poster, time);

    const body = document.createElement('div');
    body.className = 'clip-body';

    const title = document.createElement('div');
    title.className = 'clip-title';

    const when = document.createElement('strong');
    when.textContent = formatWhen(clip.createdAt);

    const cause = document.createElement('span');
    cause.className = clip.manual ? 'manual' : '';
    cause.textContent = clip.manual ? 'Manual' : 'Motion trip';

    title.append(when, cause);

    const detail = document.createElement('p');
    detail.className = 'mono';
    detail.textContent = `${clip.name} • ${clip.sizeMegabytes.toFixed(1)} MB`;

    const openExternally = async (): Promise<void> => {
        const result = await OpenExternally(clip.path);
        if (!result.ok) {
            showResult(messageBanner, result);
        }
    };

    // Clips are H.264, which the webview plays in place. One recorded with the
    // MPEG-4 fallback codec fails to decode, so it goes to the system player.
    const play = document.createElement('button');
    play.type = 'button';
    play.className = 'has-icon i-play';
    play.textContent = 'Play Clip';
    play.addEventListener('click', () => {
        const player = document.createElement('video');
        player.controls = true;
        player.autoplay = true;
        player.src = clip.url;
        player.poster = clip.posterURL;
        player.addEventListener('error', async () => {
            player.remove();
            thumb.prepend(poster);
            await openExternally();
        });
        thumb.replaceChildren(player);
    });

    const external = document.createElement('button');
    external.type = 'button';
    external.className = 'ghost has-icon i-external';
    external.title = 'Open in video player';
    external.setAttribute('aria-label', 'Open in video player');
    external.addEventListener('click', openExternally);

    body.append(title, detail, play, external);
    card.append(thumb, body);

    return card;
};

export const openRecordingsFolder = async (): Promise<void> => {
    const result = await OpenRecordingsFolder();
    if (result.ok) {
        hideBanner(messageBanner);
    } else {
        showBanner(messageBanner, 'Your recordings folder could not be opened.', 'error');
    }
};

export const registerHomeHandlers = (onStateChanged: () => void): void => {
    watchButton.addEventListener('click', async () => {
        const starting = watchButton.textContent === 'Start watching';

        busy = true;
        watchButton.disabled = true;
        setButtonPending(watchButton, true);
        watchButton.textContent = starting ? 'Turning your camera on…' : 'Stopping…';
        hideBanner(messageBanner);

        try {
            const result = await (starting ? Arm() : Disarm());

            if (result.ok) {
                hideBanner(messageBanner);
            } else {
                showResult(messageBanner, result);
            }
        } catch {
            showBanner(messageBanner, 'SpareEye could not change the camera state. Try again.', 'error');
        } finally {
            busy = false;
            await clearButtonPending(watchButton);
            onStateChanged();
        }
    });

    recordButton.addEventListener('click', async () => {
        const starting = recordButton.textContent === 'Record now';

        recordingBusy = true;
        recordButton.disabled = true;
        setButtonPending(recordButton, true);
        recordButton.textContent = starting ? 'Starting…' : 'Stopping…';
        try {
            const result = await (starting ? StartManualRecording() : StopManualRecording());
            if (result.ok) {
                showBanner(messageBanner, result.headline, 'good');
            } else {
                showResult(messageBanner, result);
            }
        } catch {
            showBanner(messageBanner, 'SpareEye could not change the recording state. Try again.', 'error');
        } finally {
            recordingBusy = false;
            await clearButtonPending(recordButton);
            recordButton.disabled = false;
            onStateChanged();
        }
    });

    snapshotButton.addEventListener('click', async () => {
        snapshotBusy = true;
        snapshotButton.disabled = true;
        setButtonPending(snapshotButton, true);
        try {
            const result = await TakeSnapshot();
            if (result.ok) {
                showBanner(messageBanner, result.headline, 'good');
            } else {
                showResult(messageBanner, result);
            }
        } catch {
            showBanner(messageBanner, 'SpareEye could not take a snapshot. Try again.', 'error');
        } finally {
            snapshotBusy = false;
            await clearButtonPending(snapshotButton);
            snapshotButton.disabled = false;
            onStateChanged();
        }
    });

    requireElement<HTMLButtonElement>('#fullscreen-button').addEventListener('click', () => {
        void viewport.requestFullscreen();
    });

    requireElement<HTMLButtonElement>('#open-media-folder').addEventListener('click', () => void openRecordingsFolder());
};
