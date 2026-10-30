import assert from 'node:assert/strict';
import test from 'node:test';
import {notificationsSchema, telegramSchema} from '../src/schemas.ts';

test('notifications accept the switch being on or off', () => {
    assert.deepEqual(notificationsSchema.parse({desktop: true}), {desktop: true});
    assert.deepEqual(notificationsSchema.parse({desktop: false}), {desktop: false});
});

test('notifications reject anything that is not a boolean', () => {
    assert.equal(notificationsSchema.safeParse({desktop: 'yes'}).success, false);
});

test('telegram accepts empty credentials, because Telegram is optional', () => {
    assert.deepEqual(
        telegramSchema.parse({botToken: '', chatID: '', enabled: true}),
        {botToken: '', chatID: '', enabled: true},
    );
});

test('telegram trims pasted credentials', () => {
    assert.deepEqual(
        telegramSchema.parse({botToken: '  123:abc \n', chatID: ' 42 ', enabled: true}),
        {botToken: '123:abc', chatID: '42', enabled: true},
    );
});
