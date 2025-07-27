// Question interactions JavaScript
document.addEventListener('DOMContentLoaded', () => {
    // Question like and dislike buttons
    const likeQuestionButtons = document.querySelectorAll('.btn-like-question');
    const dislikeQuestionButtons = document.querySelectorAll('.btn-dislike-question');
    const commentQuestionButtons = document.querySelectorAll('.btn-comment-question');

    // Function to handle question reaction response
    async function handleQuestionReactionResponse(response, questionId) {
        const contentType = response.headers.get('content-type');
        if (!contentType || !contentType.includes('application/json')) {
            // If response is not JSON, show guest prompt for specific question
            const promptContainer = document.getElementById(`reaction-prompt-${questionId}`);
            if (promptContainer) {
                promptContainer.style.display = 'block';
            }
            return null;
        }
        return await response.json();
    }

    // Handle Like Button Click for Questions
    likeQuestionButtons.forEach(button => {
        button.addEventListener('click', async () => {
            const questionId = button.getAttribute('data-id');
            
            // Check if user is authenticated
            const isAuthenticated = document.querySelector('.user-info') !== null;
            
            if (!isAuthenticated) {
                // Redirect to login if not authenticated
                window.location.href = '/login';
                return;
            }

            try {
                const response = await fetch(`/like-question?id=${questionId}`, { method: 'POST' });
                const data = await handleQuestionReactionResponse(response, questionId);
                if (data) {
                    updateQuestionReactionCounts(questionId, data.likes, data.dislikes);
                }
            } catch (err) {
                console.error('Error:', err);
            }
        });
    });

    // Handle Dislike Button Click for Questions
    dislikeQuestionButtons.forEach(button => {
        button.addEventListener('click', async () => {
            const questionId = button.getAttribute('data-id');
            
            // Check if user is authenticated
            const isAuthenticated = document.querySelector('.user-info') !== null;
            
            if (!isAuthenticated) {
                // Redirect to login if not authenticated
                window.location.href = '/login';
                return;
            }

            try {
                const response = await fetch(`/dislike-question?id=${questionId}`, { method: 'POST' });
                const data = await handleQuestionReactionResponse(response, questionId);
                if (data) {
                    updateQuestionReactionCounts(questionId, data.likes, data.dislikes);
                }
            } catch (err) {
                console.error('Error:', err);
            }
        });
    });

    // Handle Comment Button Click for Questions
    commentQuestionButtons.forEach(button => {
        button.addEventListener('click', () => {
            const questionId = button.getAttribute('data-id');
            
            // Check if we're on the dashboard page
            const isDashboard = window.location.pathname === '/dashboard' || 
                              window.location.pathname === '/';
            
            if (isDashboard) {
                // Redirect to view-question page
                window.location.href = `/view-question?id=${questionId}#comments-section`;
                return;
            }
            
            // If we're already on the view-question page, scroll to comments
            const commentsSection = document.getElementById('comments-section');
            if (commentsSection) {
                commentsSection.scrollIntoView({ behavior: 'smooth' });
                const textarea = commentsSection.querySelector('textarea');
                if (textarea) {
                    textarea.focus();
                }
            }
        });
    });
});

// Function to update question reaction counts
function updateQuestionReactionCounts(questionId, likes, dislikes) {
    // Update in dashboard view
    const dashboardLikeButtons = document.querySelectorAll(`.btn-like-question[data-id="${questionId}"]`);
    const dashboardDislikeButtons = document.querySelectorAll(`.btn-dislike-question[data-id="${questionId}"]`);
    
    dashboardLikeButtons.forEach(button => {
        button.querySelector('span').innerText = likes;
    });
    
    dashboardDislikeButtons.forEach(button => {
        button.querySelector('span').innerText = dislikes;
    });

    // Update in view-question view if we're on that page
    const viewQuestionLikeButton = document.querySelector(`.post-actions .btn-like-question[data-id="${questionId}"] span`);
    const viewQuestionDislikeButton = document.querySelector(`.post-actions .btn-dislike-question[data-id="${questionId}"] span`);
    
    if (viewQuestionLikeButton) viewQuestionLikeButton.innerText = likes;
    if (viewQuestionDislikeButton) viewQuestionDislikeButton.innerText = dislikes;
}