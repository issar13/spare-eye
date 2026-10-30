export const setButtonPending = (button: HTMLButtonElement, pending: boolean): void => {
    button.classList.toggle('is-pending', pending);

    if (pending) {
        button.setAttribute('aria-busy', 'true');
        return;
    }

    button.removeAttribute('aria-busy');
};
