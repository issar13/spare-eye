import {compareDesc, parseISO} from 'date-fns';
import {DeleteRecording, ExportRecordings, OpenExternally, SendRecording, SetProtected, Settings} from '../wailsjs/go/main/App';
import type {app} from '../wailsjs/go/models';
import {showBanner, showResult} from './banner';
import {requireElement} from './elements';
import {formatFullTimestamp, formatWhen} from './format';
import {loadRecordings} from './queries';
import {clearButtonPending, setButtonPending} from './button-state';

const collections = ['clips', 'snapshots', 'starred'] as const;
type Collection = typeof collections[number];

const tabLabels: Record<Collection, string> = {clips: 'Videos', snapshots: 'Photos', starred: 'Starred'};

const toCollection = (name: string | undefined): Collection =>
    collections.find((collection) => collection === name) ?? 'clips';

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
        render(recordingsToShow(clips, snapshots));
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

// Starred shows clips and photos together, so the two lists are merged and
// ordered by time again; each list is only newest-first on its own.
const starredRecordings = (clips: app.RecordingView[], snapshots: app.RecordingView[]): app.RecordingView[] =>
    [...clips, ...snapshots]
        .filter((recording) => recording.protected)
        .toSorted((first, second) => compareDesc(parseISO(first.createdAt), parseISO(second.createdAt)));

const recordingsToShow = (clips: app.RecordingView[], snapshots: app.RecordingView[]): app.RecordingView[] => {
    if (shown === 'starred') {
        return starredRecordings(clips, snapshots);
    }

    return shown === 'clips' ? clips : snapshots;
};

const emptyMessages: Record<Collection, string> = {
    clips: 'No videos yet. They appear here once motion has been recorded.',
    snapshots: 'No photos yet. One is saved each time motion is first noticed.',
    starred: 'No starred recordings yet. Star a clip to keep it forever.',
};

const render = (recordings: app.RecordingView[]): void => {
    if (recordings.length === 0) {
        const empty = document.createElement('p');
        empty.className = 'hint';
        empty.textContent = emptyMessages[shown];
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
    const counts: Record<Collection, number> = {
        clips: clips.length,
        snapshots: snapshots.length,
        starred: starredRecordings(clips, snapshots).length,
    };
    tabs.forEach((tab) => {
        const collection = toCollection(tab.dataset.collection);
        tab.textContent = `${tabLabels[collection]} (${counts[collection]})`;
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
    cleanup.textContent = `Recordings older than ${days} days are removed each time SpareEye starts. Starred recordings are never removed automatically.`;
    protection.textContent = `Old clips are removed automatically after ${days} days, so your laptop’s drive never fills up. Starred recordings are never removed automatically.`;
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

    const picture = document.createElement('img');
    picture.alt = recording.video
        ? `Clip recorded ${formatFullTimestamp(recording.createdAt)}`
        : `Photo taken ${formatFullTimestamp(recording.createdAt)}`;
    picture.loading = 'lazy';
    picture.addEventListener('load', () => frame.classList.remove('unavailable'));
    picture.addEventListener('error', () => {
        picture.classList.add('unavailable');
        frame.classList.add('unavailable');
    });

    if (!recording.video) {
        picture.src = recording.url;
        frame.append(picture, tag, fallback);
        return frame;
    }

    // The webview cannot decode the clip's codec, so the card shows its poster
    // and the play button hands the file to the system player instead.
    const play = document.createElement('button');
    play.type = 'button';
    play.className = 'play-button has-icon i-play';
    play.setAttribute('aria-label', `Play the clip from ${formatFullTimestamp(recording.createdAt)}`);
    play.addEventListener('click', async () => showResult(message, await OpenExternally(recording.path)));

    if (recording.posterURL) {
        picture.src = recording.posterURL;
        frame.append(picture);
    } else {
        frame.classList.add('unavailable');
    }

    frame.append(tag, play, fallback);
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

    const star = iconButton('i-star', recording.protected ? 'Unstar' : 'Star (keep forever)');
    star.classList.toggle('starred', recording.protected);
    star.setAttribute('aria-pressed', String(recording.protected));
    star.addEventListener('click', async () => {
        await runAction(star, () => SetProtected(recording.path, !recording.protected));
        await refreshRecordings();
    });

    const send = iconButton('i-send', 'Send to Telegram');
    send.addEventListener('click', () => void runAction(send, () => SendRecording(recording.path)));

    const open = iconButton('i-external', recording.video ? 'Open in video player' : 'Open in photo viewer');
    open.addEventListener('click', () => void runAction(open, () => OpenExternally(recording.path)));

    const remove = iconButton('i-trash', 'Delete');
    remove.classList.add('danger');
    remove.addEventListener('click', async () => {
        const label = formatFullTimestamp(recording.createdAt);
        const starredNote = recording.protected ? ' It is starred.' : '';
        if (!window.confirm(`Delete the recording from ${label}?${starredNote} This cannot be undone.`)) {
            return;
        }

        await runAction(remove, () => DeleteRecording(recording.path));
        await refreshRecordings();
    });

    actions.append(size, star, send, open, remove);

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
        shown = toCollection(tab.dataset.collection);
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

    // The Go side builds the file list for the tab being shown, so only its
    // name crosses over; the save dialog is native and picks the destination.
    const exportButton = requireElement<HTMLButtonElement>('#recordings-export');
    exportButton.addEventListener('click', () => void runAction(exportButton, () => ExportRecordings(shown)));
};
