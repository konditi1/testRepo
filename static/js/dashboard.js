// inline editing for posts using javascript
document.addEventListener("DOMContentLoaded", () => {
    const editButtons = document.querySelectorAll(".edit-post-btn");
    editButtons.forEach((btn) => {
        btn.addEventListener("click", (event) => {
            const postId = btn.dataset.id;
            const postCard = document.getElementById(`post-${postId}`);
            const postTitle = postCard.querySelector("h4").textContent;
            const postContent = postCard.querySelector("p").textContent;
            // Replace content with an editable form
            postCard.innerHTML = `
                <form action="/edit-post?id=${postId}" method="POST" class="edit-post-form">
                    <input type="text" name="title" value="${postTitle}" required>
                    <textarea name="content" required>${postContent}</textarea>
                    <button type="submit">Save</button>
                    <button type="button" class="cancel-edit-btn">Cancel</button>
                </form>
            `;
            // Add cancel button functionality
            const cancelButton = postCard.querySelector(".cancel-edit-btn");
            cancelButton.addEventListener("click", () => {
                postCard.innerHTML = `
                    <div class="post">
                        <h4>${postTitle}</h4>
                        <p>${postContent}</p>
                    </div>
                `;
            });
        });
    });
});

document.addEventListener('DOMContentLoaded', function() {
    // My Activities toggle functionality
    const activitiesSectionHeader = document.querySelector('.activities-section-header');
    const activitiesToggleIcon = document.querySelector('.activities-toggle-icon');
    const activitiesScrollable = document.querySelector('.activities-scrollable');
    const sidebar = document.querySelector('.sidebar');
    
    if (activitiesSectionHeader && activitiesToggleIcon && activitiesScrollable) {
        // Set initial state (collapsed by default)
        activitiesToggleIcon.classList.add('collapsed');
        activitiesScrollable.classList.remove('expanded');
        activitiesScrollable.style.maxHeight = '0';
        
        // Toggle functionality
        activitiesSectionHeader.addEventListener('click', function() {
            // Toggle classes
            activitiesScrollable.classList.toggle('expanded');
            activitiesToggleIcon.classList.toggle('collapsed');
            
            // Force redraw by triggering reflow
            void activitiesScrollable.offsetWidth;
            
            // Set appropriate max-height based on expanded state
            if (activitiesScrollable.classList.contains('expanded')) {
                // When expanded, set a proper height
                activitiesScrollable.style.maxHeight = '200px';
                
                // Ensure sidebar is scrollable
                if (sidebar) {
                    sidebar.style.overflowY = 'auto';
                }
            } else {
                // When collapsed, explicitly set max-height to zero
                activitiesScrollable.style.maxHeight = '0';
            }
            
            // Save user preference in localStorage
            localStorage.setItem('activitiesExpanded', activitiesScrollable.classList.contains('expanded'));
        });
        
        // Check localStorage for user preference
        const activitiesPreference = localStorage.getItem('activitiesExpanded');
        if (activitiesPreference !== null) {
            if (activitiesPreference === 'true') {
                // Expand if the saved preference is true
                activitiesScrollable.classList.add('expanded');
                activitiesToggleIcon.classList.remove('collapsed');
                activitiesScrollable.style.maxHeight = '200px';
                
                // Ensure sidebar is scrollable
                if (sidebar) {
                    sidebar.style.overflowY = 'auto';
                }
            } else {
                // Make sure collapsed state is properly set
                activitiesScrollable.classList.remove('expanded');
                activitiesToggleIcon.classList.add('collapsed');
                activitiesScrollable.style.maxHeight = '0';
            }
        }
    }

     // Jobs section toggle functionality
     const jobsSectionHeader = document.querySelector('.jobs-section-header');
     const jobsToggleIcon = document.querySelector('.jobs-toggle-icon');
     const jobsScrollable = document.querySelector('.jobs-scrollable');
     
     if (jobsSectionHeader && jobsToggleIcon && jobsScrollable) {
         // Set initial state (collapsed by default)
         jobsToggleIcon.classList.add('collapsed');
         jobsScrollable.classList.remove('expanded');
         jobsScrollable.style.maxHeight = '0';
         
         // Toggle functionality
         jobsSectionHeader.addEventListener('click', function() {
             // Toggle classes
             jobsScrollable.classList.toggle('expanded');
             jobsToggleIcon.classList.toggle('collapsed');
             
             // Force redraw by triggering reflow
             void jobsScrollable.offsetWidth;
             
             // Set appropriate max-height based on expanded state
             if (jobsScrollable.classList.contains('expanded')) {
                 // When expanded, set a proper height
                 jobsScrollable.style.maxHeight = '200px';
                 
                 // Ensure sidebar is scrollable
                 if (sidebar) {
                     sidebar.style.overflowY = 'auto';
                 }
             } else {
                 // When collapsed, explicitly set max-height to zero
                 jobsScrollable.style.maxHeight = '0';
             }
             
             // Save user preference in localStorage
             localStorage.setItem('jobsExpanded', jobsScrollable.classList.contains('expanded'));
         });
         
         // Check localStorage for user preference
         const jobsPreference = localStorage.getItem('jobsExpanded');
         if (jobsPreference !== null) {
             if (jobsPreference === 'true') {
                 // Expand if the saved preference is true
                 jobsScrollable.classList.add('expanded');
                 jobsToggleIcon.classList.remove('collapsed');
                 jobsScrollable.style.maxHeight = '200px';
                 
                 // Ensure sidebar is scrollable
                 if (sidebar) {
                     sidebar.style.overflowY = 'auto';
                 }
             } else {
                 // Make sure collapsed state is properly set
                 jobsScrollable.classList.remove('expanded');
                 jobsToggleIcon.classList.add('collapsed');
                 jobsScrollable.style.maxHeight = '0';
             }
         }
     }
    
    // Chat users toggle functionality
    const chatSectionHeader = document.querySelector('.chat-section-header');
    const chatToggleIcon = document.querySelector('.chat-toggle-icon');
    const chatUsersScrollable = document.querySelector('.chat-users-scrollable');
    
    if (chatSectionHeader && chatToggleIcon && chatUsersScrollable) {
        // Set initial state (collapsed by default)
        chatToggleIcon.classList.add('collapsed');
        chatUsersScrollable.classList.remove('expanded');
        chatUsersScrollable.style.maxHeight = '0';
        
        // Toggle functionality
        chatSectionHeader.addEventListener('click', function() {
            // Toggle classes
            chatUsersScrollable.classList.toggle('expanded');
            chatToggleIcon.classList.toggle('collapsed');
            
            // Force redraw by triggering reflow
            void chatUsersScrollable.offsetWidth;
            
            // Set appropriate max-height based on expanded state
            if (chatUsersScrollable.classList.contains('expanded')) {
                // When expanded, ensure proper height for scrolling
                chatUsersScrollable.style.maxHeight = '400px';
                
                // Ensure sidebar is scrollable
                if (sidebar) {
                    sidebar.style.overflowY = 'auto';
                }
            } else {
                // When collapsed, explicitly set max-height to zero
                chatUsersScrollable.style.maxHeight = '0';
            }
            
            // Save user preference in localStorage
            localStorage.setItem('chatUsersExpanded', chatUsersScrollable.classList.contains('expanded'));
        });
        
        // Check localStorage for user preference
        const userPreference = localStorage.getItem('chatUsersExpanded');
        if (userPreference !== null) {
            if (userPreference === 'true') {
                // Expand if the saved preference is true
                chatUsersScrollable.classList.add('expanded');
                chatToggleIcon.classList.remove('collapsed');
                chatUsersScrollable.style.maxHeight = '400px';
                
                // Ensure sidebar is scrollable
                if (sidebar) {
                    sidebar.style.overflowY = 'auto';
                }
            } else {
                // Make sure collapsed state is properly set
                chatUsersScrollable.classList.remove('expanded');
                chatToggleIcon.classList.add('collapsed');
                chatUsersScrollable.style.maxHeight = '0';
            }
        }
    }
    
    // Handle window resize
    window.addEventListener('resize', function() {
        const isMobile = window.innerWidth <= 920;
        
        if (chatUsersScrollable && chatToggleIcon) {
            // On mobile, collapse by default unless user has explicitly expanded
            if (isMobile && localStorage.getItem('chatUsersExpanded') !== 'true') {
                chatUsersScrollable.classList.remove('expanded');
                chatToggleIcon.classList.add('collapsed');
                chatUsersScrollable.style.maxHeight = '0';
            }
        }
        
        if (activitiesScrollable && activitiesToggleIcon) {
            // On mobile, collapse by default unless user has explicitly expanded
            if (isMobile && localStorage.getItem('activitiesExpanded') !== 'true') {
                activitiesScrollable.classList.remove('expanded');
                activitiesToggleIcon.classList.add('collapsed');
                activitiesScrollable.style.maxHeight = '0';
            }
        }
    });
});