document.addEventListener('DOMContentLoaded', () => {
    const form = document.getElementById('create-question-form');
    if (!form) return;

    form.addEventListener('submit', (e) => {
        const title = document.getElementById('title').value.trim();
        const categories = document.querySelectorAll('input[name="category[]"]:checked');
        const attachment = document.getElementById('attachment').files[0];

        // Validate title
        if (!title) {
            e.preventDefault();
            alert('Question title is required.');
            return;
        }

        // Validate categories
        if (categories.length === 0) {
            e.preventDefault();
            alert('At least one category must be selected.');
            return;
        }

        // Validate attachment size (10MB)
        if (attachment && attachment.size > 10 * 1024 * 1024) {
            e.preventDefault();
            alert('Attachment size must be less than 10MB.');
            return;
        }
    });
});