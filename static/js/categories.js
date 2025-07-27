document.addEventListener('DOMContentLoaded', () => {
    const categoryLinks = document.querySelectorAll('.category-link');
    const postsContainer = document.querySelector('.posts');

    categoryLinks.forEach(link => {
        link.addEventListener('click', async (e) => {
            e.preventDefault();
            console.log('Category link clicked'); // Debug log
            
            const category = link.getAttribute('data-category');
            console.log('Selected category:', category); // Debug log
            
            try {
                const response = await fetch(`/posts-by-category?category=${encodeURIComponent(category)}`);
                console.log('Response status:', response.status); // Debug log
                
                if (!response.ok) {
                    throw new Error(`HTTP error! status: ${response.status}`);
                }
                
                const data = await response.json();
                console.log('Received data:', data); // Debug log

                if (data.posts && Array.isArray(data.posts)) {
                    // Update posts container
                    const postsHTML = data.posts.map(post => `
                        <hr>
                        <div class="post-card" id="post-${post.ID}">
                            <div class="profile">
                                <div class="avatar">
                                    <i class="fa-regular fa-user"></i>
                                    <div class="avatar-name">
                                        <p class="username">${post.Username}</p>
                                        <p>${post.CreatedAtHuman}</p>
                                    </div>
                                </div>
                                <div class="category-tag-container">
                                    <div class="category-tag">
                                        <ul>
                                            <li>${category}</li>
                                        </ul>                                        
                                    </div>
                                    ${post.IsOwner ? `
                                        <div class="post-options-menu">
                                            <button class="btn-options" aria-label="Post options">
                                                <i class="fa-solid fa-ellipsis"></i>
                                            </button>
                                            <div class="options-dropdown" style="display: none;">
                                                <a href="/edit-post?id=${post.ID}" class="dropdown-item">
                                                    <i class="fa-regular fa-pen-to-square"></i> Edit
                                                </a>
                                                <form action="/delete-post?id=${post.ID}" method="POST">
                                                    <button type="submit" class="dropdown-item">
                                                        <i class="fa-regular fa-trash-can"></i> Delete
                                                    </button>
                                                </form>
                                            </div>
                                        </div>
                                    ` : ''}
                                </div>
                            </div>
                            <div class="post">
                                <a href="/view-post?id=${post.ID}">
                                    <h4>${post.Title}</h4>
                                    ${post.ImageURL ? `
                                        <div class="post-image">
                                            <img src="${post.ImageURL}" alt="Post Image" class="post-image-preview">
                                        </div>
                                    ` : ''}
                                    <p>${post.Preview}</p>
                                </a>
                                <div class="reaction">
                                    <i class="btn-like" data-id="${post.ID}"><i class="fa-regular fa-thumbs-up"></i> <span>${post.Likes}</span></i>
                                    <i class="btn-dislike" data-id="${post.ID}"><i class="fa-regular fa-thumbs-down"></i> <span>${post.Dislikes}</span></i>
                                    <i class="btn-comment" data-id="${post.ID}"><i class="fa-regular fa-message"></i> <span>${post.CommentsCount}</span></i>
                                    <div class="btn-share"><i class="fa-solid fa-share-nodes"></i></div>
                                </div>
                                <div class="reaction-prompt-container" id="reaction-prompt-${post.ID}" style="display: none;">
                                    <div class="guest-reaction-prompt">
                                        <p><a href="/login">Login</a> or <a href="/signup">Sign up</a> to like or dislike the discussion</p>
                                    </div>
                                </div>
                            </div>
                        </div>
                    `).join('');

                    // Update the posts container with new content
                    postsContainer.innerHTML = `
                        <h3>${category} discussions</h3>
                             <div class="create-post">
                            <a href="/create-post" class="btn-create-post">Create New Post</a>
                        </div>
                        ${postsHTML}
                    `;

                    // Reinitialize event listeners
                    initializeEventListeners();
                }
            } catch (error) {
                console.error('Error fetching posts:', error);
            }
        });
    });
});

// Helper function to reinitialize event listeners
function initializeEventListeners() {
    // Initialize dropdown functionality
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

    // Add delete confirmation
    document.querySelectorAll('.options-dropdown form').forEach(form => {
        form.addEventListener('submit', (e) => {
            if (!confirm('Are you sure you want to delete this post?')) {
                e.preventDefault();
            }
        });
    });

    // Reinitialize other event listeners (likes, comments, etc.)
    document.querySelectorAll('.btn-like').forEach(btn => btn.addEventListener('click', handleLike));
    document.querySelectorAll('.btn-dislike').forEach(btn => btn.addEventListener('click', handleDislike));
    document.querySelectorAll('.btn-comment').forEach(btn => btn.addEventListener('click', handleComment));
}

// Handler functions
function handleLike(e) {
    e.preventDefault();
    const postId = this.getAttribute('data-id');
    fetch(`/like-post?id=${postId}`, { method: 'POST' })
        .then(response => response.json())
        .then(data => updateReactionCounts(postId, data.likes, data.dislikes))
        .catch(err => console.error('Error:', err));
}

function handleDislike(e) {
    e.preventDefault();
    const postId = this.getAttribute('data-id');
    fetch(`/dislike-post?id=${postId}`, { method: 'POST' })
        .then(response => response.json())
        .then(data => updateReactionCounts(postId, data.likes, data.dislikes))
        .catch(err => console.error('Error:', err));
}

function handleEdit(e) {
    e.preventDefault();
    const postId = this.getAttribute('data-id');
    window.location.href = `/edit-post?id=${postId}`;
}

function handleDelete(e) {
    e.preventDefault();
    const postId = this.getAttribute('data-id');
    if (confirm('Are you sure you want to delete this post?')) {
        fetch(`/delete-post?id=${postId}`, { method: 'POST' })
            .then(response => {
                if (response.ok) {
                    document.getElementById(`post-${postId}`).remove();
                }
            })
            .catch(err => console.error('Error:', err));
    }
}

function handleComment(e) {
    e.preventDefault();
    const postId = this.getAttribute('data-id');
    
    // Always redirect to view-post page when comment button is clicked
    window.location.href = `/view-post?id=${postId}#comments-section`;
} 