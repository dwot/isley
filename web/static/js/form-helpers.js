// form-helpers.js
// Lightweight form UX helpers: submit button loading state and inline validation.
(() => {
    /**
     * Wire up all forms to show a spinner on their submit button while a
     * fetch request is in progress.  Call `formHelpers.withLoading(btn, promise)`
     * in your submit handler to activate the spinner and disable the button.
     */
    function withLoading(btn, promise) {
        if (!btn) return promise;
        btn.classList.add('is-loading');
        btn.disabled = true;
        return promise.finally(() => {
            btn.classList.remove('is-loading');
            btn.disabled = false;
        });
    }

    /**
     * Mark a field as invalid with a message.
     * Adds .is-invalid to the input and shows the message in a sibling .invalid-feedback div.
     */
    function setFieldError(input, message) {
        if (!input) return;
        input.classList.add('is-invalid');
        let feedback = input.parentElement.querySelector('.invalid-feedback');
        if (!feedback) {
            feedback = document.createElement('div');
            feedback.className = 'invalid-feedback';
            input.parentElement.appendChild(feedback);
        }
        feedback.textContent = message;
        feedback.style.display = 'block';
    }

    /**
     * Clear validation state from a field.
     */
    function clearFieldError(input) {
        if (!input) return;
        input.classList.remove('is-invalid');
        const feedback = input.parentElement.querySelector('.invalid-feedback');
        if (feedback) feedback.style.display = 'none';
    }

    /**
     * Clear all validation errors in a form.
     */
    function clearFormErrors(form) {
        if (!form) return;
        form.querySelectorAll('.is-invalid').forEach(el => el.classList.remove('is-invalid'));
        form.querySelectorAll('.invalid-feedback').forEach(el => el.style.display = 'none');
    }

    // Auto-clear field errors on input
    document.addEventListener('input', (e) => {
        if (e.target.classList.contains('is-invalid')) {
            clearFieldError(e.target);
        }
    });
    document.addEventListener('change', (e) => {
        if (e.target.classList.contains('is-invalid')) {
            clearFieldError(e.target);
        }
    });

    /**
     * Format a Date object as a datetime-local input value (YYYY-MM-DDTHH:MM:SS)
     * using the browser's local timezone.
     */
    function formatDateTimeLocal(date) {
        const pad = n => String(n).padStart(2, '0');
        return `${date.getFullYear()}-${pad(date.getMonth()+1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`;
    }

    /**
     * Set a datetime-local input to the current local time.
     * Can be called on page load or when a modal opens.
     */
    function setDateTimeNow(inputId) {
        const el = document.getElementById(inputId);
        if (el) el.value = formatDateTimeLocal(new Date());
    }

    window.formHelpers = { withLoading, setFieldError, clearFieldError, clearFormErrors, formatDateTimeLocal, setDateTimeNow };
})();
