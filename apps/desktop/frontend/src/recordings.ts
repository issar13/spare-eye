import {DeleteRecording, OpenExternally, SendRecording, Settings} from '../wailsjs/go/main/App';
import type {app} from '../wailsjs/go/models';
import {showBanner, showResult} from './banner';
import {requireElement} from './elements';
import {formatFullTimestamp, formatWhen} from './format';
import {loadRecordings} from './queries';
import {clearButtonPending, setButtonPending} from './button-state';

type Collection = 'clips' | 'snapshots';

const list = requireElement<HTMLElement>('#recording-list');
const message = requireElement<HTMLElement>('#recordings-message');
const total = requireElement<HTMLElement>('#recordings-total');
const summary = requireElement<HTMLElement>('#recordings-summary');
const retention = requireElement<HTMLElement>('#recordings-retention');
const retentionDays = requireElement<HTMLElement>('#recordings-days');
const cleanup = requireElement<HTMLElement>('#recordings-cleanup');
const protection = requireElement<HTMLElement>('#protection-copy');
const tabs = Array.from(document.querySelectorAll<HTMLButtonElement>('[data-collection]'));

let shown: Collection = 'clips';

export const refreshRecordings = async (): Promise<void> => {
    try {
        const [clips, snapshots, settings] = await Promise.all([
            loadRecordings('clips'),
            loadRecordings('snapshots'),
            Settings(),
        ]);

        renderUsage(clips, snapshots, settings.tuning.retentionDays);
        render(shown === 'clips' ? clips : snapshots);
    } catch {
        // The listing only fails if the recordings folder cannot be read, which
        // is a storage problem rather than anything the user did wrong. The list
        // itself is replaced as well as the banner shown: leaving it untouched
        // means a heading and three tabs above an empty region on a first visit,
        // and rows from the other tab after a later failure.
        const failed = document.createElement('p');
        failed.className = 'hint';
        failed.textContent = 'This list could not be loaded.';
        list.replaceChildren(failed);

        showBanner(
            message,
            'Your recordings folder could not be read. Check the drive it is on is still connected, ' +
            'then press Refresh.',
            'error',
        );
    }
};

const render = (recordings: app.RecordingView[]): void => {
    if (recordings.length === 0) {
        const empty = document.createElement('p');
        empty.className = 'hint';
        empty.textContent = shown === 'clips'
            ? 'No videos yet. They appear here once motion has been recorded.'
            : 'No photos yet. One is saved each time motion is first noticed.';
        list.replaceChildren(empty);
        return;
    }

    list.replaceChildren(...recordings.map(renderRecording));
};

// renderUsage sums what the Go side reported for each file, so the figure is the
// space these recordings take, not the free space left on the drive.
const renderUsage = (clips: app.RecordingView[], snapshots: app.RecordingView[], days: number): void => {
    const megabytes = [...clips, ...snapshots].reduce((sum, recording) => sum + recording.sizeMegabytes, 0);

    total.textContent = megabytes >= 1024 ? `${(megabytes / 1024).toFixed(1)} GB` : `${megabytes.toFixed(1)} MB`;
    summary.textContent = `${clips.length} videos • ${snapshots.length} photos`;
    tabs.forEach((tab) => {
        const count = tab.dataset.collection === 'snapshots' ? snapshots.length : clips.length;
        tab.textContent = `${tab.dataset.collection === 'snapshots' ? 'Photos' : 'Videos'} (${count})`;
    });

    if (days === 0) {
        retention.textContent = '• Kept until you delete them';
        retentionDays.textContent = 'Keeps everything';
        cleanup.textContent = 'Automatic cleanup is off, so recordings stay until you delete them.';
        protection.textContent = 'Automatic cleanup is off. Turn it on to stop old clips from filling your laptop’s drive.';
        return;
    }

    retention.textContent = `• Auto-clears after ${days} days`;
    retentionDays.textContent = `Auto-clears after ${days}d`;
    cleanup.textContent = `Recordings older than ${days} days are removed each time SpareEye starts.`;
    protection.textContent = `Old clips are removed automatically after ${days} days, so your laptop’s drive never fills up.`;
};

const renderRecording = (recording: app.RecordingView): HTMLElement => {
    const card = document.createElement('article');
    card.className = 'recording';

    const body = document.createElement('div');
    body.className = 'recording-body';

    const when = document.createElement('h4');
    when.textContent = formatWhen(recording.createdAt);

    const meta = document.createElement('p');
    meta.className = 'meta';
    const metaIcon = document.createElement('i');
    metaIcon.className = recording.video ? 'icon i-video' : 'icon i-image';
    meta.append(metaIcon, document.createTextNode(recording.name));

    body.append(when, meta);
    card.append(renderMedia(recording), body, renderActions(recording));

    return card;
};

const renderMedia = (recording: app.RecordingView): HTMLElement => {
    const frame = document.createElement('div');
    frame.className = 'recording-media';

    const tag = document.createElement('span');
    tag.className = recording.manual ? 'media-tag manual' : 'media-tag';
    tag.textContent = recording.manual ? 'Manual recording' : 'Motion detected';

    const fallback = document.createElement('span');
    fallback.className = 'media-placeholder';
    fallback.textContent = recording.video ? 'Video preview unavailable' : 'Photo preview unavailable';

    if (!recording.video) {
        const still = document.createElement('img');
        still.alt = `Photo taken ${formatFullTimestamp(recording.createdAt)}`;
        still.loading = 'lazy';
        still.addEventListener('load', () => frame.classList.remove('unavailable'));
        still.addEventListener('error', () => {
            still.classList.add('unavailable');
            frame.classList.add('unavailable');
        });
        still.src = recording.url;
        frame.append(still, tag, fallback);
        return frame;
    }

    const player = document.createElement('video');
    player.src = recording.url;
    player.preload = 'metadata';
    player.addEventListener('loadeddata', () => frame.classList.remove('unavailable'));
    player.addEventListener('error', () => {
        player.classList.add('unavailable');
        frame.classList.add('unavailable');
    });

    // The clip plays in place once asked to; until then the card shows its
    // first frame with a play button, rather than a row of native controls.
    const play = document.createElement('button');
    play.type = 'button';
    play.className = 'play-button has-icon i-play';
    play.setAttribute('aria-label', `Play the clip from ${formatFullTimestamp(recording.createdAt)}`);
    play.addEventListener('click', () => {
        frame.classList.add('playing');
        player.controls = true;
        void player.play();
    });

    frame.append(player, tag, play, fallback);
    return frame;
};

const iconButton = (icon: string, label: string): HTMLButtonElement => {
    const button = document.createElement('button');
    button.type = 'button';
    button.className = `icon-button has-icon ${icon}`;
    button.title = label;
    button.setAttribute('aria-label', label);
    return button;
};

const renderActions = (recording: app.RecordingView): HTMLElement => {
    const actions = document.createElement('div');
    actions.className = 'actions';

    const size = document.createElement('span');
    size.className = 'size';
    size.textContent = `${recording.sizeMegabytes.toFixed(1)} MB on this laptop`;

    const send = iconButton('i-send', 'Send to Telegram');
    send.addEventListener('click', () => void runAction(send, () => SendRecording(recording.path)));

    const open = iconButton('i-external', recording.video ? 'Open in video player' : 'Open in photo viewer');
    open.addEventListener('click', () => void runAction(open, () => OpenExternally(recording.path)));

    const remove = iconButton('i-trash', 'Delete');
    remove.classList.add('danger');
    remove.addEventListener('click', async () => {
        const label = formatFullTimestamp(recording.createdAt);
        if (!window.confirm(`Delete the recording from ${label}? This cannot be undone.`)) {
            return;
        }

        await runAction(remove, () => DeleteRecording(recording.path));
        await refreshRecordings();
    });

    actions.append(size, send, open, remove);

    return actions;
};

const runAction = async (
    button: HTMLButtonElement,
    action: () => Promise<app.ActionResult>,
): Promise<void> => {
    button.disabled = true;
    setButtonPending(button, true);

    try {
        showResult(message, await action());
    } finally {
        await clearButtonPending(button);
        button.disabled = false;
    }
};

export const registerRecordingHandlers = (): void => {
    tabs.forEach((tab) => tab.addEventListener('click', () => {
        shown = tab.dataset.collection === 'snapshots' ? 'snapshots' : 'clips';
        tabs.forEach((other) => other.classList.toggle('selected', other === tab));
        void refreshRecordings();
    }));

    const refreshButton = requireElement<HTMLButtonElement>('#recordings-refresh');
    refreshButton.addEventListener('click', async () => {
        refreshButton.disabled = true;
        setButtonPending(refreshButton, true);
        const originalLabel = refreshButton.textContent;
        refreshButton.textContent = 'Refreshing…';

        try {
            await refreshRecordings();
        } finally {
            await clearButtonPending(refreshButton);
            refreshButton.disabled = false;
            refreshButton.textContent = originalLabel;
        }
    });
};
