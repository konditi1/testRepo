document.addEventListener("DOMContentLoaded", () => {
    const errorMessage = document.querySelector('.error-message');
    const formInputs = document.querySelectorAll('input, textarea');
    let errorTimeout;

    if (errorMessage) {
        // Add fade-out class and remove error message when inputs change
        formInputs.forEach(input => {
            input.addEventListener('input', () => {
                if (errorMessage) {
                    errorMessage.classList.add('fade-out');
                    
                    // Clear any existing timeout
                    if (errorTimeout) {
                        clearTimeout(errorTimeout);
                    }
                    
                    // Remove the error message after animation completes
                    errorTimeout = setTimeout(() => {
                        errorMessage.remove();
                    }, 300); // matches animation duration
                }
            });
        });
    }
}); 