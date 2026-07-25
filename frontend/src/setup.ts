import {Arm, Cameras, RescanCameras, SaveTelegram, SelectCamera, Settings, TestTelegram} from '../wailsjs/go/main/App';
import type {app, detector} from '../wailsjs/go/models';
import {hideBanner, showBanner, showResult} from './banner';
import {requireElement, requireField} from './elements';

const telegramForm = requireElement<HTMLFormElement>('#telegram-form');
const setupMessage = requireElement<HTMLElement>('#setup-message');
const cameraMessage = requireElement<HTMLElement>('#camera-message');
const startMessage = requireElement<HTMLElement>('#start-message');
const picker = requireElement<HTMLSelectElement>('#camera-picker');
const startButton = requireElement<HTMLButtonElement>('#setup-start');

// Whether the last scan actually found the selected camera, which is what
// decides if step one shows as finished.
let cameraConfirmed = false;

const readTelegramForm = (): app.TelegramForm => ({
    botToken: requireField(telegramForm, 'botToken').value.trim(),
    chatID: requireField(telegramForm, 'chatID').value.trim(),
    enabled: requireField(telegramForm, 'enabled').checked,
});

export const loadSetup = async (): Promise<void> => {
    const settings = await Settings();

    requireField(telegramForm, 'botToken').value = settings.telegram.botToken;
    requireField(telegramForm, 'chatID').value = settings.telegram.chatID;
    requireField(telegramForm, 'enabled').checked = settings.telegram.enabled;

    renderCameraOptions([], settings.cameraIndex);
};

// renderCameraOptions always keeps the configured camera selectable, even when
// a scan did not find it: an empty picker after a webcam is unplugged would
// hide which camera the app is still trying to use.
const renderCameraOptions = (cameras: detector.Camera[], selectedIndex: number): void => {
    cameraConfirmed = cameras.some((camera) => camera.index === selectedIndex);

    const found = cameras.map((camera) => ({
        value: camera.index,
        label: `Camera ${camera.index} — ${camera.width} × ${camera.height}`,
    }));
    const missing = cameraConfirmed
        ? []
        : [{value: selectedIndex, label: `Camera ${selectedIndex} — not responding`}];

    picker.replaceChildren(...[...missing, ...found].map((option) => {
        const element = document.createElement('option');
        element.value = String(option.value);
        element.textContent = option.label;
        element.selected = option.value === selectedIndex;
        return element;
    }));
};

// look reuses the last scan; lookAgain forces a fresh sweep of every device,
// which is what the button is for.
const showCameras = async (look: () => Promise<detector.Camera[]>): Promise<void> => {
    showBanner(cameraMessage, 'Looking for cameras…');

    const [cameras, settings] = await Promise.all([look(), Settings()]);
    renderCameraOptions(cameras, settings.cameraIndex);

    if (cameras.length === 0) {
        showBanner(
            cameraMessage,
            'No camera responded. Another program is probably using it — close Zoom, Teams or OBS, ' +
            'then press Look again.',
            'error',
        );
        return;
    }

    showBanner(
        cameraMessage,
        cameras.length === 1 ? 'Found your camera.' : `Found ${cameras.length} cameras. Pick the one you want.`,
        'good',
    );
};

// updateSetupProgress ticks the steps off as they are completed, so a first-run
// user can see at a glance what is left to do.
export const updateSetupProgress = (status: app.StatusView): void => {
    markStep('#step-camera', cameraConfirmed);
    markStep('#step-telegram', status.telegramConnected || !status.alertsEnabled);
    markStep('#step-start', status.armed);

    startButton.disabled = status.armed || !status.canArm;
    startButton.textContent = status.armed ? 'Already watching' : 'Start watching';
};

const markStep = (selector: string, done: boolean): void => {
    requireElement<HTMLElement>(selector).classList.toggle('done', done);

    const state = requireElement<HTMLElement>(`${selector}-state`);
    state.textContent = done ? 'Done' : 'To do';
    state.className = done ? 'step-state done' : 'step-state';
};

export const registerSetupHandlers = (onSettingsChanged: () => void, onWatching: () => void): void => {
    telegramForm.addEventListener('submit', async (submission) => {
        submission.preventDefault();

        showResult(setupMessage, await SaveTelegram(readTelegramForm()));
        onSettingsChanged();
    });

    requireElement<HTMLButtonElement>('#test-telegram').addEventListener('click', async (event) => {
        const button = event.currentTarget as HTMLButtonElement;

        button.disabled = true;
        showBanner(setupMessage, 'Sending a test message…');

        try {
            showResult(setupMessage, await TestTelegram(readTelegramForm()));
        } finally {
            button.disabled = false;
        }

        onSettingsChanged();
    });

    requireElement<HTMLButtonElement>('#camera-refresh').addEventListener('click', () => void showCameras(RescanCameras));

    picker.addEventListener('change', async () => {
        hideBanner(cameraMessage);

        const result = await SelectCamera(Number(picker.value));
        if (!result.ok) {
            showResult(cameraMessage, result);
        }

        onSettingsChanged();
    });

    startButton.addEventListener('click', async () => {
        startButton.disabled = true;
        showBanner(startMessage, 'Turning your camera on…');

        const result = await Arm();
        if (result.ok) {
            hideBanner(startMessage);
            onWatching();
        } else {
            showResult(startMessage, result);
        }

        onSettingsChanged();
    });
};

export const activateSetup = (): void => {
    void showCameras(Cameras);
};
