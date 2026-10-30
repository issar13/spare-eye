import {DeleteRecording, OpenExternally, SendRecording} from '../wailsjs/go/main/App';
import type {app} from '../wailsjs/go/models';
import {showBanner, showResult} from './banner';
import {requireElement} from './elements';
import {formatFullTimestamp} from './format';
import {loadRecordings} from './queries';
import {setButtonPending} from './button-state';

type Collection = 'clips' | 'snapshots';

const list = requireElement<HTMLElement>('#recording-list');
const message = requireElement<HTMLElement>('#recordings-message');

let shown: Collection = 'clips';

export const refreshRecordings = async (): Promise<void> => {
    try {
        render(await loadRecordings(shown));
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

const renderRecording = (recording: app.RecordingView): HTMLElement => {
    const row = document.createElement('div');
    row.className = 'recording';

    const summary = document.createElement('div');
    summary.className = 'recording-body';

    const when = document.createElement('h4');
    when.textContent = formatFullTimestamp(recording.createdAt);

    const meta = document.createElement('p');
    meta.className = 'meta';
    meta.textContent = [
        recording.manual ? 'You started this one' : 'Started by movement',
        `${recording.sizeMegabytes.toFixed(1)} MB`,
    ].join(' · ');

    const name = document.createElement('p');
    name.className = 'meta filename';
    name.textContent = recording.name;

    summary.append(when, meta, name);
    row.append(renderMedia(recording), summary, renderActions(recording));

    return row;
};

const renderMedia = (recording: app.RecordingView): HTMLElement => {
    const frame = document.createElement('div');
    frame.className = 'recording-media';

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
        frame.append(still, fallback);
        return frame;
    }

    const player = document.createElement('video');
    player.src = recording.url;
    player.controls = true;
    player.preload = 'metadata';
    player.addEventListener('loadeddata', () => frame.classList.remove('unavailable'));
    player.addEventListener('error', () => {
        player.classList.add('unavailable');
        frame.classList.add('unavailable');
    });

    frame.append(player, fallback);
    return frame;
};

const renderActions = (recording: app.RecordingView): HTMLElement => {
    const actions = document.createElement('div');
    actions.className = 'actions';

    const send = document.createElement('button');
    send.type = 'button';
    send.textContent = 'Send to Telegram';
    send.addEventListener('click', () => void runAction(send, () => SendRecording(recording.path)));

    const open = document.createElement('button');
    open.type = 'button';
    open.textContent = recording.video ? 'Open in video player' : 'Open in photo viewer';
    open.addEventListener('click', () => void runAction(open, () => OpenExternally(recording.path)));

    const remove = document.createElement('button');
    remove.type = 'button';
    remove.className = 'danger';
    remove.textContent = 'Delete';
    remove.addEventListener('click', async () => {
        const label = formatFullTimestamp(recording.createdAt);
        if (!window.confirm(`Delete the recording from ${label}? This cannot be undone.`)) {
            return;
        }

        await runAction(remove, () => DeleteRecording(recording.path));
        await refreshRecordings();
    });

    actions.append(send, open, remove);

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
        setButtonPending(button, false);
        button.disabled = false;
    }
};

export const registerRecordingHandlers = (): void => {
    const tabs = Array.from(document.querySelectorAll<HTMLButtonElement>('[data-collection]'));

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
            setButtonPending(refreshButton, false);
            refreshButton.disabled = false;
            refreshButton.textContent = originalLabel;
        }
    });
};
