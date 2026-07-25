import type {app} from '../wailsjs/go/models';
import {hideBanner, showBanner} from './banner';
import {requireElement} from './elements';

// The detector re-encodes a frame every 200ms, so asking more often than that
// only re-fetches bytes we already have.
const frameIntervalMs = 200;

// retryIntervalMs applies when the camera is not armed: /preview.jpg answers
// 204 and the image element reports that as a load error.
const retryIntervalMs = 700;

const image = requireElement<HTMLImageElement>('#preview-image');
const idleBanner = requireElement<HTMLElement>('#preview-idle');
const caption = requireElement<HTMLElement>('#preview-caption');

let streaming = false;
let pending = 0;

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
