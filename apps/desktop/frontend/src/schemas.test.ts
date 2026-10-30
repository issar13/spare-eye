import {describe, expect, it} from 'vitest';

import {notificationsSchema, telegramSchema} from './schemas';

describe('notificationsSchema', () => {
    it('accepts the switch being on or off', () => {
        expect(notificationsSchema.parse({desktop: true})).toEqual({desktop: true});
        expect(notificationsSchema.parse({desktop: false})).toEqual({desktop: false});
    });

    it('rejects anything that is not a boolean', () => {
        expect(notificationsSchema.safeParse({desktop: 'yes'}).success).toBe(false);
    });
});

describe('telegramSchema', () => {
    it('accepts empty credentials, because Telegram is optional', () => {
        expect(telegramSchema.parse({botToken: '', chatID: '', enabled: true}))
            .toEqual({botToken: '', chatID: '', enabled: true});
    });

    it('trims pasted credentials', () => {
        expect(telegramSchema.parse({botToken: '  123:abc \n', chatID: ' 42 ', enabled: true}))
            .toEqual({botToken: '123:abc', chatID: '42', enabled: true});
    });
});
