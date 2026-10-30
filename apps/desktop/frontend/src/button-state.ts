const minimumPendingMs = 240;
const pendingStartedAt = new WeakMap<HTMLButtonElement, number>();

export const setButtonPending = (button: HTMLButtonElement, pending: boolean): void => {
    button.classList.toggle('is-pending', pending);

    if (pending) {
        pendingStartedAt.set(button, performance.now());
        button.setAttribute('aria-busy', 'true');
        return;
    }

    pendingStartedAt.delete(button);
    button.removeAttribute('aria-busy');
};

export const clearButtonPending = async (button: HTMLButtonElement): Promise<void> => {
    const startedAt = pendingStartedAt.get(button);
    if (startedAt !== undefined) {
        const remainingMs = minimumPendingMs - (performance.now() - startedAt);
        if (remainingMs > 0) {
            await new Promise((resolve) => setTimeout(resolve, remainingMs));
        }
    }

    setButtonPending(button, false);
};
