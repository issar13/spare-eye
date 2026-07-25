import type {app} from '../wailsjs/go/models';

// The detector decides motion from two numbers: how large a moving region must
// be, and how much a pixel must change to count. Neither means anything to
// somebody pointing a camera at their porch, so the UI offers three named
// levels and writes the numbers behind them.

export type SensitivityLevel = 'low' | 'medium' | 'high';

type Preset = {
    minimumContourArea: number;
    thresholdValue: number;
    label: string;
    description: string;
};

export const sensitivityPresets: Record<SensitivityLevel, Preset> = {
    low: {
        minimumContourArea: 20000,
        thresholdValue: 40,
        label: 'Only big movements',
        description: 'Ignores pets, shadows and swaying branches. You will get the fewest alerts.',
    },
    medium: {
        minimumContourArea: 8000,
        thresholdValue: 30,
        label: 'Recommended',
        description: 'Picks up a person walking into view. A good starting point for most homes.',
    },
    high: {
        minimumContourArea: 3000,
        thresholdValue: 20,
        label: 'Small movements too',
        description: 'Catches pets and small changes. Expect more alerts, including false ones.',
    },
};

export const sensitivityOrder: SensitivityLevel[] = ['low', 'medium', 'high'];

// levelFor reads the saved numbers back as a level, using bands rather than
// exact matches. The shipped day and night defaults differ slightly from each
// other, and neither should show up as some third "custom" state on first run.
export const levelFor = (profile: app.SensitivityForm): SensitivityLevel => {
    if (profile.minimumContourArea <= 5000) {
        return 'high';
    }
    if (profile.minimumContourArea <= 12000) {
        return 'medium';
    }

    return 'low';
};

export const applyLevel = (profile: app.SensitivityForm, level: SensitivityLevel): app.SensitivityForm => ({
    ...profile,
    minimumContourArea: sensitivityPresets[level].minimumContourArea,
    thresholdValue: sensitivityPresets[level].thresholdValue,
});
