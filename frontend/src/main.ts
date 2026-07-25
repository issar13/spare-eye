import './styles.css';

import {Status} from '../wailsjs/go/main/App';
import type {app} from '../wailsjs/go/models';
import {loadDetectionForm, registerDetectionHandlers} from './detection';
import {requireElement} from './elements';
import {refreshActivity, registerHomeHandlers, updateHome} from './home';
import {loadLiveViewForm, registerLiveViewHandlers, startPreview, stopPreview, updatePreview} from './preview';
import {refreshRecordings, registerRecordingHandlers} from './recordings';
import {activateSetup, loadSetup, registerSetupHandlers, updateSetupProgress} from './setup';

// Status drives the sidebar on every screen, so it is polled continuously. The
// heavier reads — the activity list, the recordings — happen only while their
// screen is on show.
const statusIntervalMs = 1000;
const activityIntervalMs = 2000;

const panelNames = ['home', 'preview', 'recordings', 'detection', 'setup'] as const;
type PanelName = (typeof panelNames)[number];

const navButtons = Array.from(document.querySelectorAll<HTMLButtonElement>('[data-panel]'));
const statusDot = requireElement<HTMLElement>('#status-dot');
const statusLabel = requireElement<HTMLElement>('#status-label');

let activityTimer = 0;

const showPanel = (name: PanelName): void => {
    navButtons.forEach((button) => button.classList.toggle('selected', button.dataset.panel === name));
    panelNames.forEach((panel) => {
        requireElement<HTMLElement>(`#panel-${panel}`).hidden = panel !== name;
    });

    window.clearInterval(activityTimer);

    if (name === 'preview') {
        startPreview();
        void loadLiveViewForm();
    } else {
        stopPreview();
    }

    if (name === 'home') {
        void refreshActivity();
        activityTimer = window.setInterval(() => void refreshActivity(), activityIntervalMs);
    }

    if (name === 'recordings') {
        void refreshRecordings();
    }

    if (name === 'setup') {
        activateSetup();
    }

    if (name === 'detection') {
        void loadDetectionForm();
    }
};

const updateSidebar = (status: app.StatusView): void => {
    statusDot.className = `dot ${status.tone}`;
    statusLabel.textContent = status.headline;
};

const pollStatus = async (): Promise<app.StatusView> => {
    const status = await Status();

    updateSidebar(status);
    updateHome(status);
    updatePreview(status);
    updateSetupProgress(status);

    return status;
};

// A first run has nothing configured, so it opens on Setup. Anyone who has
// already been through it lands on Home, where the button they want is.
const openingPanel = (status: app.StatusView): PanelName =>
    status.canArm ? 'home' : 'setup';

const start = async (): Promise<void> => {
    navButtons.forEach((button) => button.addEventListener('click', () => {
        showPanel(button.dataset.panel as PanelName);
    }));

    registerHomeHandlers(() => void pollStatus());
    registerSetupHandlers(() => void pollStatus(), () => showPanel('home'));
    registerDetectionHandlers(() => void pollStatus());
    registerLiveViewHandlers();
    registerRecordingHandlers();

    await loadSetup();
    await loadDetectionForm();

    showPanel(openingPanel(await pollStatus()));

    window.setInterval(() => void pollStatus(), statusIntervalMs);
};

void start();
