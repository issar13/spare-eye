import {SaveTuning, Settings} from '../wailsjs/go/main/App';
import {app} from '../wailsjs/go/models';
import {showBanner, showResult} from './banner';
import {requireElement, requireField} from './elements';
import {formatMinutesOfDay, parseMinutesOfDay} from './format';
import {applyLevel, levelFor, SensitivityLevel, sensitivityOrder, sensitivityPresets} from './presets';
import {describeIssue, tuningSchema} from './schemas';

const form = requireElement<HTMLFormElement>('#detection-form');
const message = requireElement<HTMLElement>('#detection-message');

// The two sensitivity profiles are held here rather than read back off the
// screen, because the friendly control shows a level while the engine needs a
// pair of numbers, and the Advanced fields may have adjusted them since.
let dayProfile: app.SensitivityForm;
let nightProfile: app.SensitivityForm;

export const loadDetectionForm = async (): Promise<void> => {
    const {tuning} = await Settings();

    dayProfile = tuning.day;
    nightProfile = tuning.night;

    renderSensitivity();
    fillAdvanced();

    requireField(form, 'nightStart').value = formatMinutesOfDay(tuning.nightStartMinutes);
    requireField(form, 'nightEnd').value = formatMinutesOfDay(tuning.nightEndMinutes);

    // Alert cooldown is stored per lighting profile, but two separate "don't
    // pester me" controls is a distinction nobody wants to make. One control
    // drives both, and it reads back from the daytime value.
    requireField(form, 'alertCooldownMinutes').value = String(Math.round(dayProfile.alertCooldownSeconds / 60));

    requireField(form, 'motionPersistSeconds').value = String(tuning.motionPersistSeconds);
    requireField(form, 'clipDurationSeconds').value = String(tuning.clipDurationSeconds);
    requireField(form, 'prebufferLeadSeconds').value = String(tuning.prebufferLeadSeconds);
    requireField(form, 'retentionDays').value = String(tuning.retentionDays);
};

const renderSensitivity = (): void => {
    renderSegmented('#day-sensitivity', levelFor(dayProfile), (level) => {
        dayProfile = applyLevel(dayProfile, level);
        renderSensitivity();
        fillAdvanced();
    });

    renderSegmented('#night-sensitivity', levelFor(nightProfile), (level) => {
        nightProfile = applyLevel(nightProfile, level);
        renderSensitivity();
        fillAdvanced();
    });
};

const renderSegmented = (
    selector: string,
    selected: SensitivityLevel,
    onChoose: (level: SensitivityLevel) => void,
): void => {
    const container = requireElement<HTMLElement>(selector);

    container.replaceChildren(...sensitivityOrder.map((level) => {
        const preset = sensitivityPresets[level];

        const choice = document.createElement('button');
        choice.type = 'button';
        choice.className = level === selected ? 'choice selected' : 'choice';
        choice.setAttribute('aria-pressed', String(level === selected));
        choice.addEventListener('click', () => onChoose(level));

        const label = document.createElement('strong');
        label.textContent = preset.label;

        const description = document.createElement('span');
        description.textContent = preset.description;

        choice.append(label, description);
        return choice;
    }));
};

const fillAdvanced = (): void => {
    requireField(form, 'day.minimumContourArea').value = String(dayProfile.minimumContourArea);
    requireField(form, 'day.thresholdValue').value = String(dayProfile.thresholdValue);
    requireField(form, 'day.sampleIntervalSeconds').value = String(dayProfile.sampleIntervalSeconds);

    requireField(form, 'night.minimumContourArea').value = String(nightProfile.minimumContourArea);
    requireField(form, 'night.thresholdValue').value = String(nightProfile.thresholdValue);
    requireField(form, 'night.sampleIntervalSeconds').value = String(nightProfile.sampleIntervalSeconds);
};

const readProfile = (prefix: string, cooldownSeconds: string) => ({
    minimumContourArea: requireField(form, `${prefix}.minimumContourArea`).value,
    thresholdValue: requireField(form, `${prefix}.thresholdValue`).value,
    sampleIntervalSeconds: requireField(form, `${prefix}.sampleIntervalSeconds`).value,
    alertCooldownSeconds: cooldownSeconds,
});

// readForm hands zod the text as typed, except for the two clock fields:
// "06:30" is a time, not a number, so date-fns turns it into the minutes since
// midnight the detector schedules on.
const readForm = () => {
    const cooldownSeconds = String(Number(requireField(form, 'alertCooldownMinutes').value) * 60);

    return tuningSchema.safeParse({
        day: readProfile('day', cooldownSeconds),
        night: readProfile('night', cooldownSeconds),
        nightStartMinutes: parseMinutesOfDay(requireField(form, 'nightStart').value),
        nightEndMinutes: parseMinutesOfDay(requireField(form, 'nightEnd').value),
        motionPersistSeconds: requireField(form, 'motionPersistSeconds').value,
        prebufferLeadSeconds: requireField(form, 'prebufferLeadSeconds').value,
        clipDurationSeconds: requireField(form, 'clipDurationSeconds').value,
        retentionDays: requireField(form, 'retentionDays').value,
    });
};

export const registerDetectionHandlers = (onSettingsChanged: () => void): void => {
    form.addEventListener('submit', async (submission) => {
        submission.preventDefault();

        const parsed = readForm();
        if (!parsed.success) {
            showBanner(message, describeIssue(parsed.error), 'error');
            return;
        }

        showBanner(message, 'Saving…');

        const result = await SaveTuning(app.TuningForm.createFrom(parsed.data));
        showResult(message, result);

        if (result.ok) {
            await loadDetectionForm();
        }

        onSettingsChanged();
    });
};
