import {
    Arm,
    Cameras,
    RescanCameras,
    SaveStartup,
    SaveTelegram,
    SelectCamera,
    SelectCameraURL,
    Settings,
    TestCameraURL,
    TestTelegram,
} from '../wailsjs/go/main/App';
import type {app, detector} from '../wailsjs/go/models';
import {hideBanner, showBanner, showResult} from './banner';
import {requireElement, requireField} from './elements';
import {cameraStreamSchema, describeIssue, startupSchema, telegramSchema} from './schemas';

const telegramForm = requireElement<HTMLFormElement>('#telegram-form');
const startupForm = requireElement<HTMLFormElement>('#startup-form');
const streamForm = requireElement<HTMLFormElement>('#stream-form');
const startupMessage = requireElement<HTMLElement>('#startup-message');
const setupMessage = requireElement<HTMLElement>('#setup-message');
const cameraMessage = requireElement<HTMLElement>('#camera-message');
const streamMessage = requireElement<HTMLElement>('#stream-message');
const startMessage = requireElement<HTMLElement>('#start-message');
const picker = requireElement<HTMLSelectElement>('#camera-picker');
const startButton = requireElement<HTMLButtonElement>('#setup-start');

// unchosen is the picker's value while nobody has picked anything. A select has
// to have some value, and an empty one is the only one that cannot be confused
// with device 0.
const unchosen = '';

// Whether the chosen camera is known to work, which is what decides if step one
// shows as finished. A scan sets it directly; until one has run, the status poll
// stands in, because the engine only reports it can arm once it has opened a
// camera itself. Without that fallback the step reads as outstanding on every
// screen but Setup, and stays that way if the camera was busy at launch.
let cameraConfirmed = false;

const readTelegramForm = (): app.TelegramForm => telegramSchema.parse({
    botToken: requireField(telegramForm, 'botToken').value,
    chatID: requireField(telegramForm, 'chatID').value,
    enabled: requireField(telegramForm, 'enabled').checked,
});

const readStartupForm = (): app.StartupForm => startupSchema.parse({
    launchAtLogin: requireField(startupForm, 'launchAtLogin').checked,
    watchOnLaunch: requireField(startupForm, 'watchOnLaunch').checked,
});

export const loadSetup = async (): Promise<void> => {
    const settings = await Settings();

    requireField(telegramForm, 'botToken').value = settings.telegram.botToken;
    requireField(telegramForm, 'chatID').value = settings.telegram.chatID;
    requireField(telegramForm, 'enabled').checked = settings.telegram.enabled;

    requireField(startupForm, 'launchAtLogin').checked = settings.startup.launchAtLogin;
    requireField(startupForm, 'watchOnLaunch').checked = settings.startup.watchOnLaunch;

    requireField(streamForm, 'cameraURL').value = settings.cameraURL;

    renderCameraOptions([], settings);
};

// usingStream and usingDevice split the three states step one can be in: no
// answer yet, a camera on this computer, or one on the network. They are asked
// about separately because a network camera is never in the scanned list.
const usingStream = (settings: app.SettingsView): boolean =>
    settings.cameraChosen && settings.cameraURL !== '';

const usingDevice = (settings: app.SettingsView): boolean =>
    settings.cameraChosen && settings.cameraURL === '';

// renderCameraOptions always keeps the chosen camera selectable, even when a
// scan did not find it: an empty picker after a webcam is unplugged would hide
// which camera the app is still trying to use.
const renderCameraOptions = (cameras: detector.Camera[], settings: app.SettingsView): void => {
    const deviceFound = cameras.some((camera) => camera.index === settings.cameraIndex);

    cameraConfirmed = usingStream(settings) || (usingDevice(settings) && deviceFound);

    const found = cameras.map((camera) => ({
        value: String(camera.index),
        label: `Camera ${camera.index} — ${camera.width} × ${camera.height}`,
    }));

    // Nothing is chosen for the user. Until they answer, the picker leads with a
    // prompt rather than resting on whichever device happens to be first.
    const waiting = settings.cameraChosen
        ? []
        : [{value: unchosen, label: 'Choose a camera…'}];

    const onTheNetwork = usingStream(settings)
        ? [{value: unchosen, label: 'A camera on your network'}]
        : [];

    const missing = usingDevice(settings) && !deviceFound
        ? [{value: String(settings.cameraIndex), label: `Camera ${settings.cameraIndex} — not responding`}]
        : [];

    const selected = usingDevice(settings) ? String(settings.cameraIndex) : unchosen;

    picker.replaceChildren(...[...waiting, ...onTheNetwork, ...missing, ...found].map((option) => {
        const element = document.createElement('option');
        element.value = option.value;
        element.textContent = option.label;
        element.selected = option.value === selected;
        return element;
    }));
};

// look reuses the last scan; RescanCameras forces a fresh sweep of every device,
// which is what the button is for.
const showCameras = async (look: () => Promise<detector.Camera[]>): Promise<void> => {
    showBanner(cameraMessage, 'Looking for cameras…');

    const [cameras, settings] = await Promise.all([look(), Settings()]);

    renderCameraOptions(cameras, settings);
    showBanner(cameraMessage, ...describeCameras(cameras, settings));
};

// describeCameras says what the list means for whoever is reading it. Nothing is
// adopted on their behalf: a camera watching a room is a decision, and the app
// helping itself to whichever device answered first is how somebody ends up
// recording a room they did not mean to.
const describeCameras = (cameras: detector.Camera[], settings: app.SettingsView): [string, 'good' | 'warn' | 'error'] => {
    if (usingStream(settings)) {
        return [`Using your network camera at ${settings.cameraURL}. Pick one from the list to change it.`, 'good'];
    }

    if (cameras.length === 0) {
        return [
            'No camera responded. Another program is probably using it — close Zoom, Teams or OBS, ' +
            'then press Look again. If your camera is on your network, use the box below instead.',
            'error',
        ];
    }

    if (!settings.cameraChosen) {
        return [
            cameras.length === 1
                ? 'Found one camera. Choose it below to start using it.'
                : `Found ${cameras.length} cameras. Choose the one you want below.`,
            'warn',
        ];
    }

    if (!cameras.some((camera) => camera.index === settings.cameraIndex)) {
        return [
            `Camera ${settings.cameraIndex}, the one you chose, did not respond. Choose another one below, ` +
            'or plug it back in and press Look again.',
            'error',
        ];
    }

    return [`Using camera ${settings.cameraIndex}. Pick a different one below if you want.`, 'good'];
};

// updateSetupProgress ticks the steps off as they are completed, so a first-run
// user can see at a glance what is left to do.
export const updateSetupProgress = (status: app.StatusView): void => {
    markStep('#step-camera', cameraConfirmed || status.canArm || status.armed);
    markStep('#step-telegram', status.telegramConnected || !status.alertsEnabled);
    markStep('#step-start', status.armed);

    startButton.disabled = status.armed || !status.canArm;
    startButton.textContent = status.armed ? 'Already watching' : 'Start watching';

    showLoginRegistration(status);
};

// The tick box says what was asked for; the status says what this computer will
// really do at the next login. Security software removing the registration, and
// the app being moved since it was made, both leave the two disagreeing, and the
// one worth showing is the one that decides whether anybody is watched.
const showLoginRegistration = (status: app.StatusView): void => {
    if (status.launchAtLogin === status.launchAtLoginRegistered) {
        return;
    }

    requireField(startupForm, 'launchAtLogin').checked = status.launchAtLoginRegistered;

    showBanner(
        startupMessage,
        status.launchAtLogin
            ? 'This computer is no longer set to open Kaa Rada when you log in. Security software ' +
            'sometimes removes that, and so does moving the app. Tick the box and save to put it back.'
            : 'This computer still opens Kaa Rada when you log in, even though that is switched off ' +
            'here. Press Save to remove it.',
        'warn',
    );
};

const markStep = (selector: string, done: boolean): void => {
    requireElement<HTMLElement>(selector).classList.toggle('done', done);

    const state = requireElement<HTMLElement>(`${selector}-state`);
    state.textContent = done ? 'Done' : 'Not done yet';
    state.className = done ? 'step-state done' : 'step-state';
};

// readStreamAddress returns the typed address, or nothing once it has told the
// user what is wrong with it.
const readStreamAddress = (): string | undefined => {
    const parsed = cameraStreamSchema.safeParse({
        cameraURL: requireField(streamForm, 'cameraURL').value,
    });

    if (!parsed.success) {
        showBanner(streamMessage, describeIssue(parsed.error), 'error');
        return undefined;
    }

    return parsed.data.cameraURL;
};

export const registerSetupHandlers = (onSettingsChanged: () => void, onWatching: () => void): void => {
    telegramForm.addEventListener('submit', async (submission) => {
        submission.preventDefault();

        showResult(setupMessage, await SaveTelegram(readTelegramForm()));
        onSettingsChanged();
    });

    startupForm.addEventListener('submit', async (submission) => {
        submission.preventDefault();

        showResult(startupMessage, await SaveStartup(readStartupForm()));
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
        // The prompt and the network-camera entry are labels, not choices.
        if (picker.value === unchosen) {
            return;
        }

        hideBanner(cameraMessage);
        hideBanner(streamMessage);

        const result = await SelectCamera(Number(picker.value));
        if (result.ok) {
            await showCameras(Cameras);
        } else {
            showResult(cameraMessage, result);
        }

        onSettingsChanged();
    });

    requireElement<HTMLButtonElement>('#stream-test').addEventListener('click', async (event) => {
        const address = readStreamAddress();
        if (address === undefined) {
            return;
        }

        const button = event.currentTarget as HTMLButtonElement;

        button.disabled = true;
        showBanner(streamMessage, 'Trying that camera…');

        try {
            showResult(streamMessage, await TestCameraURL(address));
        } finally {
            button.disabled = false;
        }
    });

    streamForm.addEventListener('submit', async (submission) => {
        submission.preventDefault();

        const address = readStreamAddress();
        if (address === undefined) {
            return;
        }

        showBanner(streamMessage, 'Trying that camera…');

        const result = await SelectCameraURL(address);
        if (result.ok) {
            hideBanner(streamMessage);
            await showCameras(Cameras);
        } else {
            showResult(streamMessage, result);
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
