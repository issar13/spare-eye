import {clearButtonPending, setButtonPending} from './button-state';
import {loadDetectionForm, saveDetection} from './detection';
import {requireElement} from './elements';
import {loadLiveViewForm, saveLiveView} from './preview';
import {loadSetup, saveStartup, saveTelegram} from './setup';

// The Settings screen saves as one page: each section keeps its own banner for
// anything it needs to say, and the bar at the bottom reports the overall result.

const settingsPanel = requireElement<HTMLElement>('#panel-settings');
const bar = requireElement<HTMLElement>('.save-bar');
const title = requireElement<HTMLElement>('#save-bar-title');
const saveButton = requireElement<HTMLButtonElement>('#save-all');
const discardButton = requireElement<HTMLButtonElement>('#discard-changes');

const showState = (text: string, dirty: boolean): void => {
    title.textContent = text;
    bar.classList.toggle('dirty', dirty);
};

export const registerSaveBar = (onSettingsChanged: () => void): void => {
    // Choosing a sensitivity level is a button press rather than an input event,
    // so clicks on the choice cards count as changes too.
    const markChanged = (): void => showState('You have unsaved changes', true);
    settingsPanel.addEventListener('input', markChanged);
    settingsPanel.addEventListener('click', (event) => {
        if (event.target instanceof Element && event.target.closest('.choice') !== null) {
            markChanged();
        }
    });

    saveButton.addEventListener('click', async () => {
        saveButton.disabled = true;
        setButtonPending(saveButton, true);
        discardButton.disabled = true;
        showState('Saving…', false);

        try {
            const outcomes = await Promise.all([saveDetection(), saveTelegram(), saveStartup(), saveLiveView()]);

            showState(
                outcomes.every(Boolean)
                    ? 'All settings saved'
                    : 'Some settings were not saved. See the message in that section.',
                !outcomes.every(Boolean),
            );
        } finally {
            await clearButtonPending(saveButton);
            saveButton.disabled = false;
            discardButton.disabled = false;
            onSettingsChanged();
        }
    });

    discardButton.addEventListener('click', async () => {
        await Promise.all([loadDetectionForm(), loadSetup(), loadLiveViewForm()]);
        showState('Changes discarded', false);
    });
};
