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
});

export const liveViewSchema = z.object({
    previewFPS: z.coerce.number().int()
        .min(1, 'Under Advanced, the live view has to update between 1 and 60 times a second.')
        .max(60, 'Under Advanced, the live view has to update between 1 and 60 times a second.'),
    previewQuality: z.coerce.number().int()
        .min(30, 'Under Advanced, the live view picture quality has to be between 30 and 95.')
        .max(95, 'Under Advanced, the live view picture quality has to be between 30 and 95.'),
});

export const telegramSchema = z.object({
    botToken: z.string().trim(),
    chatID: z.string().trim(),
    enabled: z.boolean(),
});

export const describeIssue = (error: z.ZodError): string => error.issues[0].message;
