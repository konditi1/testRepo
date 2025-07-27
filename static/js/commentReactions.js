document.addEventListener('DOMContentLoaded', () => {
    const likeButtons = document.querySelectorAll('.btn-like-comment');
    const dislikeButtons = document.querySelectorAll('.btn-dislike-comment');

    // Handle Like Button Click
    likeButtons.forEach(button => {
        button.addEventListener('click', async () => {
            const commentId = button.getAttribute('data-id');
            try {
                const response = await fetch(`/like-comment?id=${commentId}`, { 
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    }
                });
                
                if (response.ok) {
                    const data = await response.json();
                    updateCommentReactions(commentId, data.likes, data.dislikes);
                }
            } catch (err) {
                console.error('Error:', err);
            }
        });
    });

    // Handle Dislike Button Click
    dislikeButtons.forEach(button => {
        button.addEventListener('click', async () => {
            const commentId = button.getAttribute('data-id');
            try {
                const response = await fetch(`/dislike-comment?id=${commentId}`, { 
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json'
                    }
                });
                
                if (response.ok) {
                    const data = await response.json();
                    updateCommentReactions(commentId, data.likes, data.dislikes);
                }
            } catch (err) {
                console.error('Error:', err);
            }
        });
    });
});

function updateCommentReactions(commentId, likes, dislikes) {
    const likeButton = document.querySelector(`.btn-like-comment[data-id="${commentId}"] span`);
    const dislikeButton = document.querySelector(`.btn-dislike-comment[data-id="${commentId}"] span`);

    if (likeButton) likeButton.innerText = likes;
    if (dislikeButton) dislikeButton.innerText = dislikes;
} 