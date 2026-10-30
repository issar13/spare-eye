import '@fontsource-variable/inter';
import './icons.css';
import './styles.css';

import type {app} from '../wailsjs/go/models';
import {loadDetectionForm, registerDetectionHandlers} from './detection';
import {requireElement} from './elements';
import {formatFreeSpace} from './format';
import {refreshActivity, refreshRecentClips, registerHomeHandlers, updateHome} from './home';
import {loadLiveViewForm, registerLiveViewHandlers, startPreview, stopPreview, updatePreview} from './preview';
import {refreshRecordings, registerRecordingHandlers} from './recordings';
import {registerSaveBar} from './save-bar';
import {activateSetup, loadSetup, registerSetupHandlers, updateSetupProgress} from './setup';
import {loadStatus} from './queries';

// Status drives the app bar on every screen, so it is polled continuously. The
// heavier reads (the activity list, the recordings) happen only while their
// screen is on show.
const statusIntervalMs = 1000;
const activityIntervalMs = 2000;

const panelNames = ['home', 'recordings', 'settings', 'cameras'] as const;
type PanelName = (typeof panelNames)[number];

// The app bar pill has room for a word or two, so it names the tone rather than
// repeating the full headline the Home screen already shows.
const stateLabels: Record<string, string> = {
    watching: 'Watching',
    recording: 'Recording',
    motion: 'Motion',
    problem: 'Needs attention',
    idle: 'Standby',
};

const navButtons = Array.from(document.querySelectorAll<HTMLButtonElement>('[data-panel]'));
const statusPill = requireElement<HTMLElement>('#status-pill');
const statusDot = requireElement<HTMLElement>('#status-dot');
const statusLabel = requireElement<HTMLElement>('#status-label');
const powerChip = requireElement<HTMLElement>('#chip-power');
const powerIcon = requireElement<HTMLElement>('#chip-power-icon');
const powerText = requireElement<HTMLElement>('#chip-power-text');
const diskChip = requireElement<HTMLElement>('#chip-disk');
const diskText = requireElement<HTMLElement>('#chip-disk-text');
const temperatureChip = requireElement<HTMLElement>('#chip-temperature');
const temperatureIcon = requireElement<HTMLElement>('#chip-temperature-icon');
const temperatureText = requireElement<HTMLElement>('#chip-temperature-text');

// Below this a laptop on battery has little time left to keep watching.
const lowBatteryPercent = 20;
// Sustained heat above this starts to throttle the camera pipeline.
const hotCelsius = 80;

let activityTimer = 0;

const showPanel = (name: PanelName): void => {
    navButtons.forEach((button) => button.classList.toggle('selected', button.dataset.panel === name));
    panelNames.forEach((panel) => {
        requireElement<HTMLElement>(`#panel-${panel}`).hidden = panel !== name;
    });

    // Each screen opens at its top rather than wherever the last one was left.
    requireElement<HTMLElement>('.content').scrollTop = 0;
    window.clearInterval(activityTimer);

    if (name === 'home') {
        startPreview();
        void refreshActivity();
        void refreshRecentClips();
        activityTimer = window.setInterval(() => {
            void refreshActivity();
            void refreshRecentClips();
        }, activityIntervalMs);
    } else {
        stopPreview();
    }

    if (name === 'recordings') {
        void refreshRecordings();
    }

    if (name === 'cameras') {
        activateSetup();
    }

    if (name === 'settings') {
        void loadDetectionForm();
        void loadLiveViewForm();
    }
};

const updateAppBar = (status: app.StatusView): void => {
    statusPill.className = `state-pill ${status.tone}`;
    statusDot.className = `dot ${status.tone}`;
    statusLabel.textContent = stateLabels[status.tone] ?? status.headline;

    // The wire carries null for "unknown" but Wails types it as optional, so
    // `?? null` settles both into one shape to test against.
    const batteryPercent = status.batteryPercent ?? null;
    const diskFreeBytes = status.diskFreeBytes ?? null;
    const temperatureCelsius = status.temperatureCelsius ?? null;

    // A desktop on mains has no battery but is still worth saying "Plugged in"
    // for. The chip is only hidden when the machine reports neither, which a
    // real machine never does; it is there so nothing is shown as a guess.
    powerChip.hidden = batteryPercent === null && !status.onMains;
    const powerState = status.onMains ? 'Plugged in' : 'On battery';
    const isLowBattery = batteryPercent !== null && !status.onMains && batteryPercent < lowBatteryPercent;
    powerText.textContent = batteryPercent === null ? powerState : `${powerState} ${batteryPercent}%`;
    powerText.classList.toggle('tint-amber', isLowBattery);
    powerIcon.className = status.onMains ? 'icon i-zap tint-green' : `icon i-battery ${isLowBattery ? 'tint-amber' : 'tint-muted'}`;

    diskChip.hidden = diskFreeBytes === null;
    diskText.textContent = diskFreeBytes === null ? '' : formatFreeSpace(diskFreeBytes);

    temperatureChip.hidden = temperatureCelsius === null;
    temperatureText.textContent = temperatureCelsius === null ? '' : `${Math.round(temperatureCelsius)}°C`;
    temperatureIcon.className = `icon i-gauge ${temperatureCelsius !== null && temperatureCelsius >= hotCelsius ? 'tint-amber' : 'tint-muted'}`;
};

const pollStatus = async (): Promise<app.StatusView> => {
    const status = await loadStatus();

    updateAppBar(status);
    updateHome(status);
    updatePreview(status);
    updateSetupProgress(status);

    return status;
};

// Buttons that jump elsewhere say where in their data attributes: data-goto
// names a panel, data-scroll an element to bring into view, and data-click a
// button whose action they repeat.
const registerShortcuts = (): void => {
    document.querySelectorAll<HTMLElement>('[data-goto], [data-scroll], [data-click]').forEach((trigger) => {
        trigger.addEventListener('click', () => {
            const {goto, scroll, click} = trigger.dataset;

            if (goto !== undefined) {
                showPanel(goto as PanelName);
            }
            if (scroll !== undefined) {
                const target = requireElement<HTMLElement>(`#${scroll}`);
                target.scrollIntoView({behavior: 'smooth', block: 'start'});
                if (target instanceof HTMLInputElement) {
                    target.focus({preventScroll: true});
                }
            }
            if (click !== undefined) {
                requireElement<HTMLButtonElement>(`#${click}`).click();
            }
        });
    });
};

// A first run has nothing configured, so it opens on Cameras. Anyone who has
// already been through it lands on Home, where the button they want is.
const openingPanel = (status: app.StatusView): PanelName =>
    status.canArm ? 'home' : 'cameras';

const start = async (): Promise<void> => {
    navButtons.forEach((button) => button.addEventListener('click', () => {
        showPanel(button.dataset.panel as PanelName);
    }));

    registerShortcuts();
    registerHomeHandlers(() => void pollStatus());
    registerSetupHandlers(() => void pollStatus(), () => showPanel('home'));
    registerDetectionHandlers();
    registerLiveViewHandlers();
    registerRecordingHandlers();
    registerSaveBar(() => void pollStatus());

    await loadSetup();
    await loadDetectionForm();
    await loadLiveViewForm();

    showPanel(openingPanel(await pollStatus()));

    window.setInterval(() => void pollStatus(), statusIntervalMs);
};

void start();
