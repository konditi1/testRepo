document.addEventListener('DOMContentLoaded', () => {
    const commentButtons = document.querySelectorAll('.btn-comment');
    
    commentButtons.forEach(button => {
        button.addEventListener('click', () => {
            const postId = button.getAttribute('data-id');
            
            // Check if we're on the dashboard page
            const isDashboard = window.location.pathname === '/dashboard' || 
                              window.location.pathname === '/';
            
            if (isDashboard) {
                // Redirect to view-post page
                window.location.href = `/view-post?id=${postId}#comments-section`;
                return;
            }
            
            // Existing comment form logic for view-post page
            const commentForm = document.getElementById(`comment-form-${postId}`);
            const isAuthenticated = document.querySelector('.user-info') !== null;
            
            if (!isAuthenticated) {
                const guestPrompt = commentForm.querySelector('.guest-comment-prompt');
                if (guestPrompt) {
                    commentForm.style.display = commentForm.style.display === 'none' ? 'block' : 'none';
                }
                return;
            }
            
            if (commentForm.style.display === 'none') {
                document.querySelectorAll('.comment-form-container').forEach(form => {
                    form.style.display = 'none';
                });
                commentForm.style.display = 'block';
                const textarea = commentForm.querySelector('textarea');
                if (textarea) {
                    textarea.focus();
                }
            } else {
                commentForm.style.display = 'none';
            }
        });
    });

    // Edit comment functionality
    document.querySelectorAll('.btn-edit-comment').forEach(button => {
        button.addEventListener('click', () => {
            const commentId = button.getAttribute('data-id');
            const commentDiv = document.getElementById(`comment-${commentId}`);
            const contentDiv = commentDiv.querySelector('.comment-content');
            const currentContent = contentDiv.textContent.trim();

            // Replace content with editable textarea
            contentDiv.innerHTML = `
                <form class="edit-comment-form">
                    <textarea required>${currentContent}</textarea>
                    <div class="button-group">
                        <button type="submit" class="btn-save">Save</button>
                        <button type="button" class="btn-cancel">Cancel</button>
                    </div>
                </form>
            `;

            const form = contentDiv.querySelector('form');
            const textarea = form.querySelector('textarea');
            textarea.focus();

            // Handle form submission
            form.addEventListener('submit', async (e) => {
                e.preventDefault();
                const newContent = textarea.value;

                try {
                    const response = await fetch(`/edit-comment?id=${commentId}`, {
                        method: 'POST',
                        headers: {
                            'Content-Type': 'application/x-www-form-urlencoded',
                        },
                        body: `content=${encodeURIComponent(newContent)}`
                    });

                    if (response.ok) {
                        contentDiv.textContent = newContent;
                    } else {
                        alert('Failed to update comment');
                    }
                } catch (error) {
                    console.error('Error:', error);
                    alert('Failed to update comment');
                }
            });

            // Handle cancel button
            form.querySelector('.btn-cancel').addEventListener('click', () => {
                contentDiv.textContent = currentContent;
            });
        });
    });

    // Delete comment functionality
    document.querySelectorAll('.btn-delete-comment').forEach(button => {
        button.addEventListener('click', async () => {
            if (!confirm('Are you sure you want to delete this comment?')) {
                return;
            }

            const commentId = button.getAttribute('data-id');
            const commentDiv = document.getElementById(`comment-${commentId}`);

            try {
                const response = await fetch(`/delete-comment?id=${commentId}`, {
                    method: 'POST'
                });

                if (response.ok) {
                    commentDiv.remove();
                    // Update comment count
                    const countSpan = document.querySelector('.comments-count');
                    const currentCount = parseInt(countSpan.textContent);
                    countSpan.textContent = currentCount - 1;
                } else {
                    alert('Failed to delete comment');
                }
            } catch (error) {
                console.error('Error:', error);
                alert('Failed to delete comment');
            }
        });
    });
}); 