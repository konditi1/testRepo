document.addEventListener('DOMContentLoaded', () => {
    const menuToggle = document.querySelector('.mobile-menu-toggle');
    const mobileMenu = document.querySelector('.mobile-menu');
    const closeMenu = document.querySelector('.close-menu');
    const dropdownHeaders = document.querySelectorAll('.dropdown-header');
    const postsContainer = document.querySelector('.posts');

    // Toggle mobile menu
    menuToggle.addEventListener('click', () => {
        mobileMenu.classList.add('active');
    });

    closeMenu.addEventListener('click', () => {
        mobileMenu.classList.remove('active');
    });

    // Close menu when clicking outside
    document.addEventListener('click', (e) => {
        if (!mobileMenu.contains(e.target) && !menuToggle.contains(e.target)) {
            mobileMenu.classList.remove('active');
        }
    });

    // Toggle dropdowns
    dropdownHeaders.forEach(header => {
        header.addEventListener('click', () => {
            const dropdown = header.nextElementSibling;
            const icon = header.querySelector('.fa-chevron-down');
            
            dropdown.classList.toggle('active');
            icon.style.transform = dropdown.classList.contains('active') 
                ? 'rotate(180deg)' 
                : 'rotate(0deg)';
        });
    });

    // Handle category links in mobile menu
    const mobileCategoryLinks = document.querySelectorAll('.mobile-menu .category-link');
    mobileCategoryLinks.forEach(link => {
        link.addEventListener('click', async (e) => {
            e.preventDefault();
            const category = link.getAttribute('data-category');
            
            try {
                const response = await fetch(`/posts-by-category?category=${encodeURIComponent(category)}`);
                if (!response.ok) {
                    throw new Error(`HTTP error! status: ${response.status}`);
                }
                
                const data = await response.json();

                if (data.posts && Array.isArray(data.posts)) {
                    // Update posts container with the same HTML structure as in categories.js
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
                            </div>
                        </div>
                    `).join('');

                    // Update the posts container
                    postsContainer.innerHTML = `
                        <h3>${category} discussions</h3>
                        <p>Join the conversation and share your thoughts</p>
                        <div class="create-post">
                            <a href="/create-post" class="btn-create-post">Create New Post</a>
                        </div>
                        ${postsHTML}
                    `;

                    // Close the mobile menu after selecting a category
                    mobileMenu.classList.remove('active');

                    // Reinitialize event listeners for the new content
                    initializeEventListeners();
                }
            } catch (error) {
                console.error('Error fetching posts:', error);
            }
        });
    });
});

// Add the initializeEventListeners function from categories.js
function initializeEventListeners() {
    // Initialize dropdown functionality
    const optionsButtons = document.querySelectorAll('.btn-options');
    
    optionsButtons.forEach(button => {
        button.addEventListener('click', (e) => {
            e.stopPropagation();
            const dropdown = button.nextElementSibling;
            
            document.querySelectorAll('.options-dropdown').forEach(d => {
                if (d !== dropdown) {
                    d.style.display = 'none';
                }
            });
            
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

    // Reinitialize reaction buttons
    const likeButtons = document.querySelectorAll('.btn-like');
    const dislikeButtons = document.querySelectorAll('.btn-dislike');
    const commentButtons = document.querySelectorAll('.btn-comment');

    likeButtons.forEach(btn => btn.addEventListener('click', handleLike));
    dislikeButtons.forEach(btn => btn.addEventListener('click', handleDislike));
    commentButtons.forEach(btn => btn.addEventListener('click', handleComment));
} 