import {Arm, Disarm, LogLines, StartManualRecording, StopManualRecording} from '../wailsjs/go/main/App';
import type {app} from '../wailsjs/go/models';
import {hideBanner, showBanner, showResult} from './banner';
import {requireElement} from './elements';
import {formatElapsedSeconds, formatWhen} from './format';
import {guideText} from './home-guide';
import {loadRecentEvents} from './queries';
import {clearButtonPending, setButtonPending} from './button-state';

const heroDot = requireElement<HTMLElement>('#hero-dot');
const heroHeadline = requireElement<HTMLElement>('#hero-headline');
const heroExplanation = requireElement<HTMLElement>('#hero-explanation');
const heroGuide = requireElement<HTMLElement>('#hero-guide');
const messageBanner = requireElement<HTMLElement>('#home-message');
const watchButton = requireElement<HTMLButtonElement>('#watch-button');
const recordButton = requireElement<HTMLButtonElement>('#record-button');
const diagnostics = requireElement<HTMLElement>('#diagnostics');
const eventList = requireElement<HTMLElement>('#event-list');
const logView = requireElement<HTMLPreElement>('#log-view');
const activityMessage = requireElement<HTMLElement>('#activity-message');

// busy suppresses the poll's button updates while starting or stopping is in
// flight. Opening a camera takes a second or two, and letting the poll reset
// the label mid-flight would make the button flicker.
let busy = false;
let recordingBusy = false;

export const updateHome = (status: app.StatusView): void => {
    heroDot.className = `hero-dot ${status.tone}`;
    heroHeadline.textContent = status.headline;
    heroExplanation.textContent = status.explanation;

    if (!busy) {
        watchButton.textContent = status.armed ? 'Stop watching' : 'Start watching';
        watchButton.disabled = !status.armed && !status.canArm;
    }

    if (!recordingBusy) {
        recordButton.disabled = !status.armed;
        recordButton.textContent = status.recordingManual ? 'Stop recording' : 'Record now';
    }

    renderGuide(status.problem);
    renderDiagnostics(status);
};

// renderGuide points at the written walkthrough when one would help. The
// obstacle itself is already the hero headline, and its remedy the explanation
// underneath, so only the pointer to the guide is left to add.
const renderGuide = (problem: app.Problem | null | undefined): void => {
    const text = guideText(problem);

    heroGuide.hidden = text === '';
    heroGuide.textContent = text;
};

// renderDiagnostics is the only place the technical numbers appear, and it
// lives behind a collapsed disclosure on the Home screen.
const renderDiagnostics = (status: app.StatusView): void => {
    const entries: [string, string][] = [
        ['Camera', status.cameraSource],
        ['Picture size', status.frameWidth === 0 ? 'Not measured yet' : `${status.frameWidth} × ${status.frameHeight}`],
        ['Frames per second', status.captureFPS === 0 ? 'Not measured yet' : status.captureFPS.toFixed(1)],
        ['Lighting profile', status.night ? 'Night' : 'Daylight'],
        ['Moving for', status.motionSeconds === 0 ? 'Nothing moving' : formatElapsedSeconds(status.motionSeconds)],
        ['Telegram', describeAlerts(status)],
        ['Recordings folder', status.mediaDirectory],
        ['Log file', status.logPath],
    ];

    if (status.lastFailure !== '') {
        entries.push(['Last engine error', status.lastFailure]);
    }

    diagnostics.replaceChildren(...entries.map(([term, value]) => {
        const card = document.createElement('dl');
        card.className = 'stat';

        const label = document.createElement('dt');
        label.textContent = term;

        const detail = document.createElement('dd');
        detail.textContent = value;

        card.append(label, detail);
        return card;
    }));
};

const describeAlerts = (status: app.StatusView): string => {
    if (!status.alertsEnabled) {
        return 'Turned off';
    }
    if (!status.alertsConfigured) {
        return 'Not set up (optional)';
    }

    return status.telegramConnected ? 'Connected' : 'Connecting…';
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
};
