import {SaveLiveView, Settings} from '../wailsjs/go/main/App';
import {app} from '../wailsjs/go/models';
import {hideBanner, showBanner, showResult} from './banner';
import {requireElement, requireField} from './elements';
import {LiveViewLevel, liveViewLevelFor, liveViewOrder, liveViewPresets} from './presets';
import {describeIssue, liveViewSchema} from './schemas';
import {renderSegmented} from './segmented';

// Matches previewInterval in internal/detector: asking faster than the
// detector re-encodes only re-fetches bytes we already have, and asking slower
// throws away frames it already paid to produce.
const frameIntervalMs = 66;

// retryIntervalMs applies when the camera is not armed: /preview.jpg answers
// 204 and the image element reports that as a load error.
const retryIntervalMs = 700;

const image = requireElement<HTMLImageElement>('#preview-image');
const idleBanner = requireElement<HTMLElement>('#preview-idle');
const caption = requireElement<HTMLElement>('#preview-caption');
const form = requireElement<HTMLFormElement>('#live-view-form');
const message = requireElement<HTMLElement>('#live-view-message');

let streaming = false;
let pending = 0;

// Held here rather than read back off the screen, because the control shows a
// named level while the detector wants a frame rate and a quality, and the
// Advanced fields may have moved them since.
let liveViewLevel: LiveViewLevel;

// requestFrame cache-busts on every load. Polling one URL is what the webview
// handles best: a multipart stream would keep a connection open that the
// asset server has no way to interrupt when the camera is released.
const requestFrame = (): void => {
    if (!streaming) {
        return;
    }

    image.src = `/preview.jpg?frame=${Date.now()}`;
};

const scheduleFrame = (delayMs: number): void => {
    window.clearTimeout(pending);
    pending = window.setTimeout(requestFrame, delayMs);
};

image.addEventListener('load', () => scheduleFrame(frameIntervalMs));
image.addEventListener('error', () => scheduleFrame(retryIntervalMs));

export const startPreview = (): void => {
    streaming = true;
    requestFrame();
};

export const stopPreview = (): void => {
    streaming = false;
    window.clearTimeout(pending);
};

export const updatePreview = (status: app.StatusView): void => {
    image.hidden = !status.armed;

    if (status.armed) {
        hideBanner(idleBanner);
        caption.textContent = status.recording
            ? 'Recording this to a video clip right now.'
            : 'This picture is not being saved unless something moves.';
        return;
    }

    showBanner(idleBanner, 'Your camera is off. Press Start watching on the Home screen to turn it on.');
    caption.textContent = '';
};

export const loadLiveViewForm = async (): Promise<void> => {
    const {liveView} = await Settings();

    liveViewLevel = liveViewLevelFor(liveView.previewFPS);
    renderLiveView();

    requireField(form, 'previewFPS').value = String(liveView.previewFPS);
    requireField(form, 'previewQuality').value = String(liveView.previewQuality);
};

const renderLiveView = (): void => {
    renderSegmented('#live-view-quality', liveViewOrder, liveViewPresets, liveViewLevel, (level) => {
        liveViewLevel = level;
        requireField(form, 'previewFPS').value = String(liveViewPresets[level].previewFPS);
        requireField(form, 'previewQuality').value = String(liveViewPresets[level].previewQuality);
        renderLiveView();
    });
};

export const registerLiveViewHandlers = (): void => {
    form.addEventListener('submit', async (submission) => {
        submission.preventDefault();

        const parsed = liveViewSchema.safeParse({
            previewFPS: requireField(form, 'previewFPS').value,
            previewQuality: requireField(form, 'previewQuality').value,
        });
        if (!parsed.success) {
            showBanner(message, describeIssue(parsed.error), 'error');
            return;
        }

        showBanner(message, 'Saving…');

        const result = await SaveLiveView(app.LiveViewForm.createFrom(parsed.data));
        showResult(message, result);

        if (result.ok) {
            await loadLiveViewForm();
        }
    });
};
