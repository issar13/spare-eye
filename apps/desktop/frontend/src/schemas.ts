import {z} from 'zod';

// Form inputs are the boundary where text somebody typed becomes engine
// settings. The Go service checks the same values again, because a webview is
// not a trustworthy validator.
//
// Every message here is written to be read on its own, next to the form. There
// is no field path in front of it: "day.sampleIntervalSeconds" names a variable,
// not anything on screen.

const sensitivitySchema = z.object({
    minimumContourArea: z.coerce.number().positive(
        'Under Advanced, the smallest movement that counts must be more than 0 pixels.',
    ),
    thresholdValue: z.coerce.number()
        .min(1, 'Under Advanced, how much a pixel must change has to be between 1 and 255.')
        .max(255, 'Under Advanced, how much a pixel must change has to be between 1 and 255.'),
    sampleIntervalSeconds: z.coerce.number().positive(
        'Under Advanced, the seconds between checks of the picture must be more than 0.',
    ),
    alertCooldownSeconds: z.coerce.number().nonnegative(
        'The number of minutes between alerts cannot be negative.',
    ),
});

const minutesOfDaySchema = z.number('Pick a time for when night mode starts and ends.').int().min(0).max(1439);

export const tuningSchema = z.object({
    day: sensitivitySchema,
    night: sensitivitySchema,
    nightStartMinutes: minutesOfDaySchema,
    nightEndMinutes: minutesOfDaySchema,
    motionPersistSeconds: z.coerce.number().positive(
        'Movement has to last at least 1 second before a recording starts.',
    ),
    prebufferLeadSeconds: z.coerce.number().nonnegative(
        'The seconds to include from before the movement cannot be negative.',
    ),
    clipDurationSeconds: z.coerce.number().positive(
        'Recordings need to be at least 1 second long.',
    ),
    retentionDays: z.coerce.number().int().nonnegative(
        'The number of days to keep recordings cannot be negative. Use 0 to keep them forever.',
    ),
    maxStorageGigabytes: z.coerce.number().nonnegative(
        'The most disk space recordings may use cannot be negative. Use 0 for no limit.',
    ),
});

export const liveViewSchema = z.object({
    previewFPS: z.coerce.number().int()
        .min(1, 'Under Advanced, the live view has to update between 1 and 60 times a second.')
        .max(60, 'Under Advanced, the live view has to update between 1 and 60 times a second.'),
    previewQuality: z.coerce.number().int()
        .min(30, 'Under Advanced, the live view picture quality has to be between 30 and 95.')
        .max(95, 'Under Advanced, the live view picture quality has to be between 30 and 95.'),
});

export const startupSchema = z.object({
    launchAtLogin: z.boolean(),
    watchOnLaunch: z.boolean(),
});

export const notificationsSchema = z.object({
    desktop: z.boolean(),
});

export const telegramSchema = z.object({
    botToken: z.string().trim(),
    chatID: z.string().trim(),
    enabled: z.boolean(),
});

// A network camera's address goes straight to a capture backend, so the shape
// of it is checked before anything is asked to open it. The Go service checks
// the same four schemes again; this one only saves the round trip.
const openableSchemes = ['rtsp:', 'rtsps:', 'http:', 'https:'];

const openableAddress = (address: string): boolean => {
    try {
        return openableSchemes.includes(new URL(address).protocol);
    } catch {
        return false;
    }
};

export const cameraStreamSchema = z.object({
    cameraURL: z.string().trim()
        .min(1, 'Type the address of your network camera first.')
        .refine(
            openableAddress,
            'That address will not work. It has to start with rtsp://, rtsps://, http:// or https:// and ' +
            'then name the camera, as in rtsp://192.168.1.50:554/stream.',
        ),
});

export const describeIssue = (error: z.ZodError): string => error.issues[0].message;
