import {PreviewFrame, SaveLiveView, Settings} from '../wailsjs/go/main/App';
import {app} from '../wailsjs/go/models';
import {hideBanner, showBanner, showResult} from './banner';
import {requireElement, requireField} from './elements';
import {LiveViewLevel, liveViewLevelFor, liveViewOrder, liveViewPresets} from './presets';
import {describeIssue, liveViewSchema} from './schemas';
import {renderSegmented} from './segmented';
import {clearButtonPending, setButtonPending} from './button-state';

// Matches previewInterval in internal/detector: asking faster than the
// detector re-encodes only re-fetches bytes we already have, and asking slower
// throws away frames it already paid to produce.
const frameIntervalMs = 66;

// retryIntervalMs applies while the camera is starting or has no frame yet.
const retryIntervalMs = 700;

const image = requireElement<HTMLImageElement>('#preview-image');
const idleBanner = requireElement<HTMLElement>('#preview-idle');
const frameMessage = requireElement<HTMLElement>('#preview-frame-message');
const caption = requireElement<HTMLElement>('#preview-caption');
const placeholder = requireElement<HTMLElement>('#preview-placeholder');
const placeholderTitle = requireElement<HTMLElement>('#preview-placeholder-title');
const placeholderCopy = requireElement<HTMLElement>('#preview-placeholder-copy');
const liveChip = requireElement<HTMLElement>('#preview-live-chip');
const form = requireElement<HTMLFormElement>('#live-view-form');
const message = requireElement<HTMLElement>('#live-view-message');

let streaming = false;
let pending = 0;
let cameraArmed = false;
let frameReady = false;
let requestInFlight = false;
let missingPreviewFrameCount = 0;

// Held here rather than read back off the screen, because the control shows a
// named level while the detector wants a frame rate and a quality, and the
// Advanced fields may have moved them since.
let liveViewLevel: LiveViewLevel;

// requestFrame uses Wails' application bridge so the preview does not depend
// on the webview routing an asset request back to the Go asset handler.
const requestFrame = async (): Promise<void> => {
    if (!streaming || requestInFlight) {
        return;
    }

    requestInFlight = true;

    try {
        const frame = await PreviewFrame();
        if (!streaming) {
            return;
        }

        if (frame === '') {
            missingPreviewFrameCount++;
            if (cameraArmed && missingPreviewFrameCount >= 5) {
                showBanner(
                    frameMessage,
                    'The camera is on, but no picture has reached the preview. Stop watching and start it again.',
                    'warn',
                );
            }
            scheduleFrame(retryIntervalMs);
            return;
        }

        missingPreviewFrameCount = 0;
        image.src = `data:image/jpeg;base64,${frame}`;
        image.hidden = !cameraArmed;
        placeholder.hidden = cameraArmed;
        liveChip.hidden = !cameraArmed;
        frameReady = true;
        hideBanner(frameMessage);
        scheduleFrame(frameIntervalMs);
    } catch {
        if (cameraArmed) {
            showBanner(frameMessage, 'The camera picture is unavailable. Trying again…', 'warn');
        }
        scheduleFrame(retryIntervalMs);
    } finally {
        requestInFlight = false;
    }
};

const scheduleFrame = (delayMs: number): void => {
    window.clearTimeout(pending);
    pending = window.setTimeout(requestFrame, delayMs);
};

image.addEventListener('error', () => {
    frameReady = false;
    missingPreviewFrameCount = 0;
    image.hidden = true;
    placeholder.hidden = false;
    liveChip.hidden = true;
    if (cameraArmed) {
        showBanner(frameMessage, 'The camera picture is unavailable. Trying again…', 'warn');
    }
    scheduleFrame(retryIntervalMs);
});

export const startPreview = (): void => {
    streaming = true;
    requestFrame();
};

export const stopPreview = (): void => {
    streaming = false;
    window.clearTimeout(pending);
    requestInFlight = false;
};

export const updatePreview = (status: app.StatusView): void => {
    cameraArmed = status.armed;
    image.hidden = !cameraArmed || !frameReady;
    placeholder.hidden = cameraArmed && frameReady;
    liveChip.hidden = !cameraArmed || !frameReady;

    if (cameraArmed) {
        hideBanner(idleBanner);
        placeholderTitle.textContent = 'Connecting to camera';
        placeholderCopy.textContent = 'Waiting for the next live picture from your camera.';
        caption.textContent = status.recording
            ? 'Recording this to a video clip right now.'
            : 'This picture is not being saved unless something moves.';
        return;
    }

    frameReady = false;
    placeholderTitle.textContent = 'Camera standby';
    placeholderCopy.textContent = 'Start watching to see the live picture here.';
    hideBanner(frameMessage);
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

        const saveButton = form.querySelector<HTMLButtonElement>('button[type="submit"]');
        if (saveButton === null || saveButton.disabled) {
            return;
        }

        saveButton.disabled = true;
        setButtonPending(saveButton, true);
        const originalLabel = saveButton.textContent;
        saveButton.textContent = 'Saving…';

        try {
            showBanner(message, 'Saving live view settings…');
            const result = await SaveLiveView(app.LiveViewForm.createFrom(parsed.data));
            showResult(message, result);

            if (result.ok) {
                await loadLiveViewForm();
            }
        } catch {
            showBanner(message, 'Could not save live view settings. Try again.', 'error');
        } finally {
            await clearButtonPending(saveButton);
            saveButton.disabled = false;
            saveButton.textContent = originalLabel;
        }
    });
};
