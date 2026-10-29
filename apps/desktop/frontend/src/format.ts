import {
    format,
    formatDistanceStrict,
    getHours,
    getMinutes,
    isToday,
    isValid,
    isYesterday,
    parse,
    parseISO,
    startOfDay,
} from 'date-fns';

// A reference day for turning minutes-since-midnight into a clock face and
// back. Only the time part is ever read from it.
const referenceDay = (): Date => startOfDay(new Date());

// formatWhen is what the activity list shows. 'p' is date-fns's localised time,
// so it reads "10:42 PM" or "22:42" according to the machine's own locale
// rather than forcing one on everybody. A bare time is ambiguous once an entry
// is a day old, and a full date on every line is noise.
export const formatWhen = (isoTimestamp: string): string => {
    const moment = parseISO(isoTimestamp);

    if (isToday(moment)) {
        return format(moment, 'p');
    }
    if (isYesterday(moment)) {
        return `Yesterday, ${format(moment, 'p')}`;
    }

    return format(moment, 'PP, p');
};

export const formatFullTimestamp = (isoTimestamp: string): string =>
    format(parseISO(isoTimestamp), 'PP, p');

export const formatElapsedSeconds = (seconds: number): string =>
    formatDistanceStrict(new Date(0), new Date(Math.round(seconds) * 1000));

// formatMinutesOfDay renders the night window boundaries the detector stores as
// minutes since midnight, which is what an <input type="time"> expects.
export const formatMinutesOfDay = (minutesSinceMidnight: number): string => {
    const asDate = referenceDay();
    asDate.setHours(Math.floor(minutesSinceMidnight / 60), minutesSinceMidnight % 60, 0, 0);

    return format(asDate, 'HH:mm');
};

// parseMinutesOfDay reads a time input back. It returns null for anything that
// is not a clock time, which is the one thing the browser lets through empty.
export const parseMinutesOfDay = (clockText: string): number | null => {
    const parsed = parse(clockText, 'HH:mm', referenceDay());
    if (!isValid(parsed)) {
        return null;
    }

    return getHours(parsed) * 60 + getMinutes(parsed);
};
