document.addEventListener('DOMContentLoaded', () => {
    const searchForm = document.getElementById('search-form');
    const searchInput = document.getElementById('search-input');
    const suggestionsContainer = document.createElement('div');
    suggestionsContainer.className = 'search-suggestions';
    searchInput.parentElement.appendChild(suggestionsContainer);

    // Handle form submission
    if (searchForm) {
        searchForm.addEventListener('submit', (event) => {
            event.preventDefault();
            const query = searchInput.value.trim();
            if (query.length < 3) {
                alert('Search query must be at least 3 characters long.');
                return;
            }
            suggestionsContainer.style.display = 'none';
            window.location.href = `/search?q=${encodeURIComponent(query)}`;
        });
    }

    // Handle input for suggestions
    let debounceTimeout;
    searchInput.addEventListener('input', () => {
        clearTimeout(debounceTimeout);
        const query = searchInput.value.trim();

        if (query.length < 3) {
            suggestionsContainer.style.display = 'none';
            suggestionsContainer.innerHTML = '';
            return;
        }

        debounceTimeout = setTimeout(async () => {
            try {
                const response = await fetch(`/api/search-suggestions?q=${encodeURIComponent(query)}`);
                if (!response.ok) {
                    throw new Error('Failed to fetch suggestions');
                }
                const suggestions = await response.json();

                suggestionsContainer.innerHTML = '';
                if (suggestions.length === 0) {
                    suggestionsContainer.style.display = 'none';
                    return;
                }

                suggestions.forEach(suggestion => {
                    const div = document.createElement('div');
                    div.className = 'suggestion-item';
                    div.textContent = suggestion.length > 50 ? suggestion.slice(0, 47) + '...' : suggestion;
                    div.addEventListener('click', () => {
                        searchInput.value = suggestion;
                        suggestionsContainer.style.display = 'none';
                        window.location.href = `/search?q=${encodeURIComponent(suggestion)}`;
                    });
                    suggestionsContainer.appendChild(div);
                });

                suggestionsContainer.style.display = 'block';
            } catch (error) {
                console.error('Error fetching suggestions:', error);
                suggestionsContainer.style.display = 'none';
            }
        }, 300);
    });

    // Hide suggestions when clicking outside
    document.addEventListener('click', (event) => {
        if (!searchInput.contains(event.target) && !suggestionsContainer.contains(event.target)) {
            suggestionsContainer.style.display = 'none';
        }
    });

    // Hide suggestions on input focus if empty
    searchInput.addEventListener('focus', () => {
        if (searchInput.value.trim().length < 3) {
            suggestionsContainer.style.display = 'none';
        }
    });
});