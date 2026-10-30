import {
    Arm,
    Cameras,
    DiscoverNetworkCameras,
    PreviewFrame,
    RescanCameras,
    SaveCameraLocation,
    SaveNotifications,
    SaveStartup,
    SaveTelegram,
    SelectCamera,
    SelectCameraURL,
    Settings,
    TestCameraURL,
    TestTelegram,
} from '../wailsjs/go/main/App';
import type {app, detector} from '../wailsjs/go/models';
import type {onvif} from '../wailsjs/go/models';
import {hideBanner, showBanner, showResult} from './banner';
import {clearButtonPending, setButtonPending} from './button-state';
import {requireElement, requireField} from './elements';
import {
    cameraLocationSchema,
    cameraStreamSchema,
    describeIssue,
    notificationsSchema,
    startupSchema,
    telegramSchema,
} from './schemas';

const telegramForm = requireElement<HTMLFormElement>('#telegram-form');
const startupForm = requireElement<HTMLFormElement>('#startup-form');
const notificationsForm = requireElement<HTMLFormElement>('#notifications-form');
const streamForm = requireElement<HTMLFormElement>('#stream-form');
const startupMessage = requireElement<HTMLElement>('#startup-message');
const notificationsMessage = requireElement<HTMLElement>('#notifications-message');
const setupMessage = requireElement<HTMLElement>('#setup-message');
const cameraMessage = requireElement<HTMLElement>('#camera-message');
const streamMessage = requireElement<HTMLElement>('#stream-message');
const networkCameraMessage = requireElement<HTMLElement>('#network-camera-message');
const networkCameraList = requireElement<HTMLUListElement>('#network-camera-list');
const startMessage = requireElement<HTMLElement>('#start-message');
const cameraCards = requireElement<HTMLElement>('#camera-cards');
const startButton = requireElement<HTMLButtonElement>('#setup-start');
const camerasDot = requireElement<HTMLElement>('#cameras-dot');
const camerasState = requireElement<HTMLElement>('#cameras-state');
const camerasFps = requireElement<HTMLElement>('#cameras-fps');
const settingsCameraDot = requireElement<HTMLElement>('#settings-camera-dot');
const settingsCameraSummary = requireElement<HTMLElement>('#settings-camera-summary');
const locationForm = requireElement<HTMLFormElement>('#location-form');
const locationChips = requireElement<HTMLElement>('#location-chips');
const locationMessage = requireElement<HTMLElement>('#location-message');

const suggestedLocations = ['Living Room', 'Front Door', 'Backyard', 'Garage', 'Baby Crib'];

let startBusy = false;

// The latest status, so the camera cards can say which camera is watching
// without waiting for the next poll.
let lastStatus: app.StatusView | undefined;

// Set by registerSetupHandlers: picking a camera changes what the app bar and
// Home screen should say, so they are refreshed straight away.
let notifySettingsChanged = (): void => {};

const readTelegramForm = (): app.TelegramForm => telegramSchema.parse({
    botToken: requireField(telegramForm, 'botToken').value,
    chatID: requireField(telegramForm, 'chatID').value,
    enabled: requireField(telegramForm, 'enabled').checked,
});

const readStartupForm = (): app.StartupForm => startupSchema.parse({
    launchAtLogin: requireField(startupForm, 'launchAtLogin').checked,
    watchOnLaunch: requireField(startupForm, 'watchOnLaunch').checked,
});

const readNotificationsForm = (): app.NotificationsForm => notificationsSchema.parse({
    desktop: requireField(notificationsForm, 'desktop').checked,
});

export const loadSetup = async (): Promise<void> => {
    const settings = await Settings();

    requireField(telegramForm, 'botToken').value = settings.telegram.botToken;
    requireField(telegramForm, 'chatID').value = settings.telegram.chatID;
    requireField(telegramForm, 'enabled').checked = settings.telegram.enabled;

    requireField(startupForm, 'launchAtLogin').checked = settings.startup.launchAtLogin;
    requireField(startupForm, 'watchOnLaunch').checked = settings.startup.watchOnLaunch;

    requireField(notificationsForm, 'desktop').checked = settings.notifications.desktop;

    requireField(streamForm, 'cameraURL').value = settings.cameraURL;
    requireField(locationForm, 'cameraLocation').value = settings.cameraLocation;
    renderLocationChips();

    renderCameraCards([], settings, '');
    renderSettingsSummary(settings);
};

// usingStream and usingDevice split the three states the camera choice can be
// in: no answer yet, a camera on this computer, or one on the network. They are
// asked about separately because a network camera is never in the scanned list.
const usingStream = (settings: app.SettingsView): boolean =>
    settings.cameraChosen && settings.cameraURL !== '';

const usingDevice = (settings: app.SettingsView): boolean =>
    settings.cameraChosen && settings.cameraURL === '';

const cameraName = (camera: detector.Camera): string =>
    camera.label === '' ? `Camera ${camera.index}` : camera.label;

const renderSettingsSummary = (settings: app.SettingsView): void => {
    settingsCameraDot.className = settings.cameraChosen ? 'dot watching' : 'dot';

    if (usingStream(settings)) {
        settingsCameraSummary.textContent = `Using your network camera at ${settings.cameraURL}.${locationSentence(settings)}`;
    } else if (usingDevice(settings)) {
        settingsCameraSummary.textContent = `Using camera ${settings.cameraIndex} plugged into this laptop.${locationSentence(settings)}`;
    } else {
        settingsCameraSummary.textContent = 'No camera chosen yet. Pick one on the Cameras screen.';
    }
};

const locationSentence = (settings: app.SettingsView): string =>
    settings.cameraLocation === '' ? '' : ` Pointed at ${settings.cameraLocation}.`;

// The chip matching what is typed shows as selected, so a chip and the input
// never disagree about the current choice.
const renderLocationChips = (): void => {
    const typedLocation = requireField(locationForm, 'cameraLocation').value.trim();

    locationChips.replaceChildren(...suggestedLocations.map((location) => {
        const chip = document.createElement('button');
        chip.type = 'button';
        chip.className = 'location-chip';
        chip.textContent = location;
        chip.setAttribute('aria-pressed', String(location === typedLocation));
        chip.addEventListener('click', () => {
            requireField(locationForm, 'cameraLocation').value = location;
            renderLocationChips();
        });

        return chip;
    }));
};

type CardState = 'watching' | 'in-use' | 'available' | 'missing';

type CardDetails = {
    title: string;
    subtitle: string;
    footer: string;
    resolution: string;
    state: CardState;
    choose?: () => Promise<void>;
};

const stateLabels: Record<CardState, string> = {
    'watching': 'WATCHING',
    'in-use': 'CHOSEN • STANDBY',
    'available': 'AVAILABLE',
    'missing': 'NOT RESPONDING',
};

// renderCameraCards always keeps the chosen camera on screen, even when a scan
// did not find it: an empty list after a webcam is unplugged would hide which
// camera the app is still trying to use. frame is a live picture for the chosen
// camera when it is watching, and empty otherwise.
const renderCameraCards = (cameras: detector.Camera[], settings: app.SettingsView, frame: string): void => {
    const armed = lastStatus?.armed ?? false;
    const deviceFound = cameras.some((camera) => camera.index === settings.cameraIndex);

    const network: CardDetails[] = usingStream(settings)
        ? [{
            title: 'Network camera',
            subtitle: 'Local RTSP / HTTP stream',
            footer: settings.cameraURL,
            resolution: lastStatus?.frameWidth ? `${lastStatus.frameWidth} × ${lastStatus.frameHeight}` : 'Measured once watching',
            state: armed ? 'watching' : 'in-use',
        }]
        : [];

    const missing: CardDetails[] = usingDevice(settings) && !deviceFound
        ? [{
            title: `Camera ${settings.cameraIndex}`,
            subtitle: 'The camera you chose',
            footer: `Device ${settings.cameraIndex}`,
            resolution: 'No picture',
            state: 'missing',
        }]
        : [];

    const found: CardDetails[] = cameras.map((camera) => {
        const chosen = usingDevice(settings) && camera.index === settings.cameraIndex;

        return {
            title: cameraName(camera),
            subtitle: 'Built-in or USB camera',
            footer: `Device ${camera.index}`,
            resolution: `${camera.width} × ${camera.height}`,
            state: chosen ? (armed ? 'watching' : 'in-use') : 'available',
            choose: chosen ? undefined : () => chooseCamera(camera.index),
        };
    });

    const cards = [...network, ...missing, ...found];

    if (cards.length === 0) {
        const empty = document.createElement('p');
        empty.className = 'hint';
        empty.textContent = 'No cameras to show yet. Press Rescan USB, or connect a network camera below.';
        cameraCards.replaceChildren(empty);
        return;
    }

    cameraCards.replaceChildren(...cards.map((card) => renderCameraCard(card, card.state === 'watching' ? frame : '', settings.cameraLocation)));
};

const renderCameraCard = (details: CardDetails, frame: string, location: string): HTMLElement => {
    const card = document.createElement('article');
    card.className = details.state === 'watching' || details.state === 'in-use' ? 'camera-card active' : 'camera-card';

    const thumb = document.createElement('div');
    thumb.className = 'camera-thumb';

    if (frame === '') {
        const ring = document.createElement('span');
        ring.className = 'standby-ring';
        const inner = document.createElement('span');
        const icon = document.createElement('i');
        icon.className = details.state === 'missing' ? 'icon i-x' : 'icon i-moon';
        inner.append(icon);
        ring.append(inner);
        thumb.append(ring);
    } else {
        const picture = document.createElement('img');
        picture.alt = `Live view from ${details.title}`;
        picture.src = `data:image/jpeg;base64,${frame}`;
        thumb.append(picture);
    }

    const top = document.createElement('div');
    top.className = 'thumb-top';

    const statePill = document.createElement('span');
    statePill.className = details.state === 'watching'
        ? 'thumb-pill live'
        : details.state === 'missing' ? 'thumb-pill off' : 'thumb-pill';
    const stateDot = document.createElement('span');
    stateDot.className = details.state === 'watching' ? 'dot watching' : 'dot';
    statePill.append(stateDot, document.createTextNode(stateLabels[details.state]));
    top.append(statePill);

    const bottom = document.createElement('span');
    bottom.className = 'thumb-bottom';
    const deviceIcon = document.createElement('i');
    deviceIcon.className = 'icon i-video';
    bottom.append(deviceIcon, document.createTextNode(details.footer));

    thumb.append(top, bottom);

    const body = document.createElement('div');
    body.className = 'camera-body';

    const heading = document.createElement('div');
    const title = document.createElement('h4');
    title.textContent = details.title;
    const subtitle = document.createElement('p');
    subtitle.className = 'subtitle';
    subtitle.textContent = details.subtitle;
    heading.append(title, subtitle);

    const resolution = document.createElement('div');
    resolution.className = 'kv-row';
    const resolutionLabel = document.createElement('span');
    const resolutionIcon = document.createElement('i');
    resolutionIcon.className = 'icon i-frame tint-cyan';
    resolutionLabel.append(resolutionIcon, document.createTextNode('Resolution'));
    const resolutionValue = document.createElement('span');
    resolutionValue.textContent = details.resolution;
    resolution.append(resolutionLabel, resolutionValue);

    // Only the camera in use has a room; the others on the list are not pointed
    // at anything the user has named.
    const showsLocation = location !== '' && (details.state === 'watching' || details.state === 'in-use');
    const locationRow = document.createElement('div');
    locationRow.className = 'kv-row';
    locationRow.hidden = !showsLocation;
    const locationLabel = document.createElement('span');
    const locationIcon = document.createElement('i');
    locationIcon.className = 'icon i-frame tint-green';
    locationLabel.append(locationIcon, document.createTextNode('Location'));
    const locationValue = document.createElement('span');
    locationValue.textContent = location;
    locationRow.append(locationLabel, locationValue);

    const action = document.createElement('button');
    action.type = 'button';

    if (details.choose !== undefined) {
        const choose = details.choose;
        action.className = 'has-icon i-eye';
        action.textContent = 'Use this camera';
        action.addEventListener('click', () => void choose());
    } else if (details.state === 'missing') {
        action.className = 'has-icon i-refresh';
        action.textContent = 'Look again';
        action.addEventListener('click', () => void showCameras(RescanCameras));
    } else {
        action.className = 'in-use has-icon i-check-circle';
        action.textContent = details.state === 'watching' ? 'In use • watching' : 'In use';
        action.disabled = true;
    }

    body.append(heading, resolution, locationRow, action);
    card.append(thumb, body);

    return card;
};

const chooseCamera = async (index: number): Promise<void> => {
    hideBanner(cameraMessage);
    hideBanner(streamMessage);

    const result = await SelectCamera(index);
    if (result.ok) {
        await showCameras(Cameras);
    } else {
        showResult(cameraMessage, result);
    }

    notifySettingsChanged();
};

// look reuses the last scan; RescanCameras forces a fresh sweep of every device,
// which is what the button is for.
const showCameras = async (look: () => Promise<detector.Camera[]>): Promise<void> => {
    showBanner(cameraMessage, 'Looking for cameras…');

    const [cameras, settings] = await Promise.all([look(), Settings()]);
    const frame = lastStatus?.armed ? await PreviewFrame() : '';

    renderCameraCards(cameras, settings, frame);
    renderSettingsSummary(settings);
    showBanner(cameraMessage, ...describeCameras(cameras, settings));
};

// describeCameras says what the list means for whoever is reading it. Nothing is
// adopted on their behalf: a camera watching a room is a decision, and the app
// helping itself to whichever device answered first is how somebody ends up
// recording a room they did not mean to.
const describeCameras = (cameras: detector.Camera[], settings: app.SettingsView): [string, 'good' | 'warn' | 'error'] => {
    if (usingStream(settings)) {
        return [`Using your network camera at ${settings.cameraURL}. Pick a camera below to change it.`, 'good'];
    }

    if (cameras.length === 0) {
        return [
            'No camera responded. Another program is probably using it. Close Zoom, Teams or OBS, ' +
            'then press Rescan USB. If your camera is on your network, use the box below instead.',
            'error',
        ];
    }

    if (!settings.cameraChosen) {
        return [
            cameras.length === 1
                ? 'Found one camera. Press Use this camera to start using it.'
                : `Found ${cameras.length} cameras. Press Use this camera on the one you want.`,
            'warn',
        ];
    }

    if (!cameras.some((camera) => camera.index === settings.cameraIndex)) {
        return [
            `Camera ${settings.cameraIndex}, the one you chose, did not respond. Choose another one below, ` +
            'or plug it back in and press Rescan USB.',
            'error',
        ];
    }

    return [`Using camera ${settings.cameraIndex}. Pick a different one below if you want.`, 'good'];
};

export const updateSetupProgress = (status: app.StatusView): void => {
    lastStatus = status;

    startButton.disabled = startBusy || status.armed || !status.canArm;
    startButton.textContent = startBusy
        ? 'Turning your camera on…'
        : status.armed ? 'Already watching' : 'Start watching';

    camerasDot.className = `dot ${status.tone}`;
    camerasState.textContent = status.armed ? 'Camera loop active' : 'Camera standby';
    camerasFps.textContent = status.captureFPS === 0
        ? 'Frame rate measured once watching'
        : `${status.captureFPS.toFixed(1)} fps • ${status.frameWidth} × ${status.frameHeight}`;

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
            ? 'This computer is no longer set to open SpareEye when you log in. Security software ' +
            'sometimes removes that, and so does moving the app. Tick the box and save to put it back.'
            : 'This computer still opens SpareEye when you log in, even though that is switched off ' +
            'here. Press Save to remove it.',
        'warn',
    );
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

// saveTelegram, saveStartup and saveNotifications are run by the Settings save bar alongside the
// other sections. Each reports into its own banner and says whether it went
// through.
export const saveTelegram = async (): Promise<boolean> => {
    try {
        const result = await SaveTelegram(readTelegramForm());
        if (result.ok) {
            hideBanner(setupMessage);
        } else {
            showResult(setupMessage, result);
        }
        return result.ok;
    } catch {
        showBanner(setupMessage, 'Could not save Telegram settings. Try again.', 'error');
        return false;
    }
};

export const saveStartup = async (): Promise<boolean> => {
    try {
        const result = await SaveStartup(readStartupForm());
        if (result.ok) {
            hideBanner(startupMessage);
        } else {
            showResult(startupMessage, result);
        }
        return result.ok;
    } catch {
        showBanner(startupMessage, 'Could not save startup settings. Try again.', 'error');
        return false;
    }
};

export const saveNotifications = async (): Promise<boolean> => {
    try {
        const result = await SaveNotifications(readNotificationsForm());
        if (result.ok) {
            hideBanner(notificationsMessage);
        } else {
            showResult(notificationsMessage, result);
        }
        return result.ok;
    } catch {
        showBanner(notificationsMessage, 'Could not save the notification setting. Try again.', 'error');
        return false;
    }
};

export const registerSetupHandlers = (onSettingsChanged: () => void, onWatching: () => void): void => {
    notifySettingsChanged = onSettingsChanged;

    // A discovered chat belongs to the bot token that produced it. Clear the
    // hidden value as soon as the token is edited so every subsequent action,
    // including a plain preference save, carries an honest unpaired state.
    requireField(telegramForm, 'botToken').addEventListener('input', () => {
        requireField(telegramForm, 'chatID').value = '';
    });

    telegramForm.addEventListener('submit', (submission) => submission.preventDefault());
    startupForm.addEventListener('submit', (submission) => submission.preventDefault());

    requireElement<HTMLButtonElement>('#reveal-token').addEventListener('click', (event) => {
        const button = event.currentTarget as HTMLButtonElement;
        const token = requireField(telegramForm, 'botToken');
        const hidden = token.type === 'password';

        token.type = hidden ? 'text' : 'password';
        button.textContent = hidden ? 'Hide' : 'Reveal';
    });

    notificationsForm.addEventListener('submit', (submission) => submission.preventDefault());

    requireElement<HTMLButtonElement>('#test-telegram').addEventListener('click', async (event) => {
        const button = event.currentTarget as HTMLButtonElement;

        button.disabled = true;
        setButtonPending(button, true);
        const originalLabel = button.textContent;
        button.textContent = 'Testing…';
        showBanner(setupMessage, 'Looking for the message you sent your bot…');

        try {
            const result = await TestTelegram(readTelegramForm());
            showResult(setupMessage, result);
            if (result.ok) {
                await loadSetup();
            }
        } finally {
            await clearButtonPending(button);
            button.disabled = false;
            button.textContent = originalLabel;
        }

        onSettingsChanged();
    });

    locationForm.addEventListener('submit', async (submission) => {
        submission.preventDefault();

        const parsed = cameraLocationSchema.safeParse(requireField(locationForm, 'cameraLocation').value);
        if (!parsed.success) {
            showBanner(locationMessage, describeIssue(parsed.error), 'error');
            return;
        }

        const saveButton = locationForm.querySelector<HTMLButtonElement>('button[type="submit"]');
        if (saveButton === null || saveButton.disabled) {
            return;
        }

        saveButton.disabled = true;
        setButtonPending(saveButton, true);

        try {
            const result = await SaveCameraLocation(parsed.data);
            showResult(locationMessage, result);
            if (result.ok) {
                requireField(locationForm, 'cameraLocation').value = parsed.data;
                renderLocationChips();
                await showCameras(Cameras);
            }
        } catch {
            showBanner(locationMessage, 'Could not save the camera location. Try again.', 'error');
        } finally {
            await clearButtonPending(saveButton);
            saveButton.disabled = false;
            onSettingsChanged();
        }
    });

    requireField(locationForm, 'cameraLocation').addEventListener('input', renderLocationChips);

    const cameraRefreshButton = requireElement<HTMLButtonElement>('#camera-refresh');
    cameraRefreshButton.addEventListener('click', async () => {
        cameraRefreshButton.disabled = true;
        setButtonPending(cameraRefreshButton, true);
        const originalLabel = cameraRefreshButton.textContent;
        cameraRefreshButton.textContent = 'Looking…';

        try {
            await showCameras(RescanCameras);
        } finally {
            await clearButtonPending(cameraRefreshButton);
            cameraRefreshButton.disabled = false;
            cameraRefreshButton.textContent = originalLabel;
        }
    });

    requireElement<HTMLButtonElement>('#network-camera-search').addEventListener('click', async (event) => {
        const searchButton = event.currentTarget as HTMLButtonElement;
        searchButton.disabled = true;
        setButtonPending(searchButton, true);
        const originalLabel = searchButton.textContent;
        searchButton.textContent = 'Searching…';
        showBanner(networkCameraMessage, 'Searching this network for ONVIF cameras…');
        networkCameraList.replaceChildren();

        try {
            const cameras = await DiscoverNetworkCameras();
            renderNetworkCameras(cameras);
            showBanner(
                networkCameraMessage,
                cameras.length === 0
                    ? 'No ONVIF cameras replied. Check that ONVIF is enabled and the camera is on this local network.'
                    : `Found ${cameras.length} ONVIF ${cameras.length === 1 ? 'camera' : 'cameras'}.`,
                cameras.length === 0 ? 'warn' : 'good',
            );
        } catch {
            showBanner(
                networkCameraMessage,
                'Could not search this network. Check that local network access is available and try again.',
                'error',
            );
        } finally {
            await clearButtonPending(searchButton);
            searchButton.disabled = false;
            searchButton.textContent = originalLabel;
        }
    });

    requireElement<HTMLButtonElement>('#stream-test').addEventListener('click', async (event) => {
        const address = readStreamAddress();
        if (address === undefined) {
            return;
        }

        const button = event.currentTarget as HTMLButtonElement;

        button.disabled = true;
        setButtonPending(button, true);
        const originalLabel = button.textContent;
        button.textContent = 'Testing…';
        showBanner(streamMessage, 'Trying that camera…');

        try {
            showResult(streamMessage, await TestCameraURL(address));
        } finally {
            await clearButtonPending(button);
            button.disabled = false;
            button.textContent = originalLabel;
        }
    });

    streamForm.addEventListener('submit', async (submission) => {
        submission.preventDefault();

        const address = readStreamAddress();
        if (address === undefined) {
            return;
        }

        const useCameraButton = streamForm.querySelector<HTMLButtonElement>('button[type="submit"]');
        if (useCameraButton === null || useCameraButton.disabled) {
            return;
        }

        useCameraButton.disabled = true;
        setButtonPending(useCameraButton, true);
        const originalLabel = useCameraButton.textContent;
        useCameraButton.textContent = 'Trying camera…';

        try {
            showBanner(streamMessage, 'Trying that camera…');
            const result = await SelectCameraURL(address);
            if (result.ok) {
                hideBanner(streamMessage);
                await showCameras(Cameras);
            } else {
                showResult(streamMessage, result);
            }

            onSettingsChanged();
        } catch {
            showBanner(streamMessage, 'Could not select that camera. Try again.', 'error');
        } finally {
            await clearButtonPending(useCameraButton);
            useCameraButton.disabled = false;
            useCameraButton.textContent = originalLabel;
        }
    });

    startButton.addEventListener('click', async () => {
        startBusy = true;
        startButton.disabled = true;
        setButtonPending(startButton, true);
        showBanner(startMessage, 'Turning your camera on…');

        try {
            const result = await Arm();
            if (result.ok) {
                hideBanner(startMessage);
                onWatching();
            } else {
                showResult(startMessage, result);
            }
        } catch {
            showBanner(startMessage, 'SpareEye could not start the camera. Try again.', 'error');
        } finally {
            startBusy = false;
            await clearButtonPending(startButton);
            onSettingsChanged();
        }
    });
};

const renderNetworkCameras = (cameras: onvif.Camera[]): void => {
    networkCameraList.replaceChildren(...cameras.map((camera) => {
        const item = document.createElement('li');
        const title = document.createElement('strong');
        const address = document.createElement('span');
        const serviceAddress = document.createElement('code');

        title.textContent = camera.name;
        address.textContent = camera.address;
        serviceAddress.textContent = camera.serviceURL;
        item.append(title, address, serviceAddress);

        return item;
    }));
};

export const activateSetup = (): void => {
    void showCameras(Cameras);
};
