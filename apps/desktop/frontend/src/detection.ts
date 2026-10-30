import {SaveTuning, Settings} from '../wailsjs/go/main/App';
import {app} from '../wailsjs/go/models';
import {hideBanner, showBanner, showResult} from './banner';
import {requireElement, requireField} from './elements';
import {formatMinutesOfDay, parseMinutesOfDay} from './format';
import {applyLevel, levelFor, sensitivityOrder, sensitivityPresets} from './presets';
import {describeIssue, tuningSchema} from './schemas';
import {renderSegmented} from './segmented';

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
    renderSegmented('#day-sensitivity', sensitivityOrder, sensitivityPresets, levelFor(dayProfile), (level) => {
        dayProfile = applyLevel(dayProfile, level);
        renderSensitivity();
        fillAdvanced();
    });

    renderSegmented('#night-sensitivity', sensitivityOrder, sensitivityPresets, levelFor(nightProfile), (level) => {
        nightProfile = applyLevel(nightProfile, level);
        renderSensitivity();
        fillAdvanced();
    });
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

// saveDetection is run by the Settings save bar alongside the other sections. It
// reports into its own banner and tells the bar whether it went through.
export const saveDetection = async (): Promise<boolean> => {
    const parsed = readForm();
    if (!parsed.success) {
        showBanner(message, describeIssue(parsed.error), 'error');
        return false;
    }

    try {
        const result = await SaveTuning(app.TuningForm.createFrom(parsed.data));
        if (result.ok) {
            hideBanner(message);
            await loadDetectionForm();
        } else {
            showResult(message, result);
        }
        return result.ok;
    } catch (error) {
        showBanner(message, error instanceof Error ? error.message : 'Could not save motion settings.', 'error');
        return false;
    }
};

export const registerDetectionHandlers = (): void => {
    form.addEventListener('submit', (submission) => submission.preventDefault());
};
