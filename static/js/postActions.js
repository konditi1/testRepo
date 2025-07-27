// likes and  dislike buttons functionalities
document.addEventListener('DOMContentLoaded', () => {
    // Only add event listeners if user is authenticated
    const likeButtons = document.querySelectorAll('.btn-like');
    const dislikeButtons = document.querySelectorAll('.btn-dislike');

    if (likeButtons.length > 0) {
        // Function to handle reaction response
        async function handleReactionResponse(response, postId) {
            const contentType = response.headers.get('content-type');
            if (!contentType || !contentType.includes('application/json')) {
                // If response is not JSON, show guest prompt for specific post
                const promptContainer = document.getElementById(`reaction-prompt-${postId}`);
                if (promptContainer) {
                    promptContainer.style.display = 'block';
                }
                return null;
            }
            return await response.json();
        }

        // Handle Like Button Click
        likeButtons.forEach(button => {
            button.addEventListener('click', async () => {
                const postId = button.getAttribute('data-id');
                const promptContainer = document.getElementById(`reaction-prompt-${postId}`);
                
                // Check if user is authenticated
                const isAuthenticated = document.querySelector('.user-info') !== null;
                
                if (!isAuthenticated) {
                    // Show guest prompt if not authenticated
                    promptContainer.style.display = 'block';
                    return;
                }

                try {
                    const response = await fetch(`/like-post?id=${postId}`, { method: 'POST' });
                    const data = await handleReactionResponse(response, postId);
                    if (data) {
                        updateReactionCounts(postId, data.likes, data.dislikes);
                    }
                } catch (err) {
                    console.error('Error:', err);
                }
            });
        });

        // Handle Dislike Button Click
        dislikeButtons.forEach(button => {
            button.addEventListener('click', async () => {
                const postId = button.getAttribute('data-id');
                const promptContainer = document.getElementById(`reaction-prompt-${postId}`);
                
                // Check if user is authenticated
                const isAuthenticated = document.querySelector('.user-info') !== null;
                
                if (!isAuthenticated) {
                    // Show guest prompt if not authenticated
                    promptContainer.style.display = 'block';
                    return;
                }

                try {
                    const response = await fetch(`/dislike-post?id=${postId}`, { method: 'POST' });
                    const data = await handleReactionResponse(response, postId);
                    if (data) {
                        updateReactionCounts(postId, data.likes, data.dislikes);
                    }
                } catch (err) {
                    console.error('Error:', err);
                }
            });
        });

        // Close prompts when clicking outside
        document.addEventListener('click', (e) => {
            if (!e.target.closest('.btn-like') && 
                !e.target.closest('.btn-dislike') && 
                !e.target.closest('.guest-reaction-prompt')) {
                document.querySelectorAll('.reaction-prompt-container').forEach(container => {
                    container.style.display = 'none';
                });
            }
        });
    }

    // Handle post options dropdown
    const optionsButtons = document.querySelectorAll('.btn-options');
    
    optionsButtons.forEach(button => {
        button.addEventListener('click', (e) => {
            e.stopPropagation();
            const dropdown = button.nextElementSibling;
            
            // Close all other dropdowns first
            document.querySelectorAll('.options-dropdown').forEach(d => {
                if (d !== dropdown) {
                    d.style.display = 'none';
                }
            });
            
            // Toggle current dropdown
            dropdown.style.display = dropdown.style.display === 'none' ? 'block' : 'none';
        });
    });

    // Close dropdown when clicking outside
    document.addEventListener('click', (e) => {
        if (!e.target.closest('.post-options-menu')) {
            document.querySelectorAll('.options-dropdown').forEach(dropdown => {
                dropdown.style.display = 'none';
            });
        }
    });

    // Add confirmation for delete action
    document.querySelectorAll('.options-dropdown form').forEach(form => {
        form.addEventListener('submit', (e) => {
            if (!confirm('Are you sure you want to delete this post?')) {
                e.preventDefault();
            }
        });
    });
});

// Function to update all instances of a post's reaction counts
function updateReactionCounts(postId, likes, dislikes) {
    // Update in dashboard view
    const dashboardLikeButtons = document.querySelectorAll(`.btn-like[data-id="${postId}"]`);
    const dashboardDislikeButtons = document.querySelectorAll(`.btn-dislike[data-id="${postId}"]`);

    dashboardLikeButtons.forEach(button => {
        button.querySelector('span').innerText = likes;
    });

    dashboardDislikeButtons.forEach(button => {
        button.querySelector('span').innerText = dislikes;
    });

    // Update in view-post view if we're on that page
    const viewPostLikeButton = document.querySelector(`.post-actions .btn-like[data-id="${postId}"] span`);
    const viewPostDislikeButton = document.querySelector(`.post-actions .btn-dislike[data-id="${postId}"] span`);

    if (viewPostLikeButton) viewPostLikeButton.innerText = likes;
    if (viewPostDislikeButton) viewPostDislikeButton.innerText = dislikes;
}
