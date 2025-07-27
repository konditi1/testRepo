document.addEventListener('DOMContentLoaded', function() {
    // Community section toggle functionality
    const expertSectionHeader = document.querySelector('.sidebar-section-header');
    const expertToggleIcon = document.querySelector('.expert-toggle-icon');
    const communityContent = document.querySelector('.community-content');
    
    if (expertSectionHeader && expertToggleIcon && communityContent) {
        // Set initial state (collapsed by default)
        expertToggleIcon.classList.add('collapsed');
        communityContent.classList.remove('expanded');
        
        // Toggle functionality
        expertSectionHeader.addEventListener('click', function() {
            communityContent.classList.toggle('expanded');
            expertToggleIcon.classList.toggle('collapsed');
            
            // Load community data when expanded for the first time
            if (communityContent.classList.contains('expanded') && !communityContent.dataset.loaded) {
                loadCommunityData();
                communityContent.dataset.loaded = 'true';
            }
            
            // Save user preference
            localStorage.setItem('communityExpanded', communityContent.classList.contains('expanded'));
        });
        
        // Check localStorage for user preference
        const communityPreference = localStorage.getItem('communityExpanded');
        if (communityPreference === 'true') {
            communityContent.classList.add('expanded');
            expertToggleIcon.classList.remove('collapsed');
            loadCommunityData();
            communityContent.dataset.loaded = 'true';
        }
    }
    
    // Load community data
    async function loadCommunityData() {
        try {
            // Load leaderboard
            await loadLeaderboard();
            
            // Load recent badges
            await loadRecentBadges();
            
            // Load user progress (if logged in)
            const isLoggedIn = document.body.dataset.isLoggedIn === 'true';
            if (isLoggedIn) {
                await loadUserProgress();
            }
            
            // Update community stats
            updateCommunityStats();
            
        } catch (error) {
            console.error('Error loading community data:', error);
        }
    }
    
    // Load leaderboard data
    async function loadLeaderboard() {
        const container = document.getElementById('leaderboard-container');
        if (!container) return;
        
        try {
            const response = await fetch('/api/community-stats');
            const data = await response.json();
            
            if (data.top_contributors && data.top_contributors.length > 0) {
                container.innerHTML = data.top_contributors.map((user, index) => `
                    <div class="leaderboard-user" data-user-id="${user.id}">
                        <span class="leaderboard-rank rank-${index + 1}">#${index + 1}</span>
                        <div class="leaderboard-avatar">
                            ${user.profile_url 
                                ? `<img src="${user.profile_url}" alt="${user.username}">`
                                : `<i class="fa-regular fa-user"></i>`
                            }
                        </div>
                        <div class="leaderboard-info">
                            <div class="leaderboard-name">${user.username}</div>
                            <span class="leaderboard-level" style="background-color: ${user.level_color}; color: white;">
                                ${user.level}
                            </span>
                        </div>
                        <div class="leaderboard-stats">
                            <div class="leaderboard-points">${user.reputation_points}</div>
                            <div class="leaderboard-contributions">${user.total_contributions} contributions</div>
                        </div>
                    </div>
                `).join('');
            } else {
                container.innerHTML = '<div class="no-data">No contributors yet</div>';
            }
        } catch (error) {
            console.error('Error loading leaderboard:', error);
            container.innerHTML = '<div class="error-state">Failed to load leaderboard</div>';
        }
    }
    
    // Load recent badges
    async function loadRecentBadges() {
        const container = document.getElementById('recent-badges-container');
        if (!container) return;
        
        try {
            const response = await fetch('/api/community-stats');
            const data = await response.json();
            
            if (data.recent_badges && data.recent_badges.length > 0) {
                container.innerHTML = data.recent_badges.map(badge => `
                    <div class="badge-award">
                        <div class="badge-icon" style="background-color: ${badge.color};">
                            <i class="${badge.icon}"></i>
                        </div>
                        <div class="badge-details">
                            <div class="badge-name">${badge.badge_name}</div>
                            <div class="badge-recipient">@${badge.username}</div>
                            <div class="badge-time">${badge.earned_at_human}</div>
                        </div>
                    </div>
                `).join('');
            } else {
                container.innerHTML = '<div class="no-data">No recent achievements</div>';
            }
        } catch (error) {
            console.error('Error loading recent badges:', error);
            container.innerHTML = '<div class="error-state">Failed to load achievements</div>';
        }
    }
    
    // Load user progress
    async function loadUserProgress() {
        const container = document.getElementById('user-progress-container');
        if (!container) return;
        
        try {
            const response = await fetch('/api/user-stats');
            const data = await response.json();
            
            if (data.user_stats) {
                const stats = data.user_stats;
                const badges = data.badges || [];
                
                // Calculate next level progress
                const levels = [
                    { name: 'Newcomer', min: 0, max: 50, color: '#6b7280' },
                    { name: 'Beginner', min: 50, max: 200, color: '#10b981' },
                    { name: 'Intermediate', min: 200, max: 500, color: '#3b82f6' },
                    { name: 'Advanced', min: 500, max: 1000, color: '#8b5cf6' },
                    { name: 'Expert', min: 1000, max: null, color: '#eab308' }
                ];
                
                const currentLevel = levels.find(level => 
                    stats.reputation_points >= level.min && 
                    (level.max === null || stats.reputation_points < level.max)
                );
                
                let progressPercent = 0;
                let nextLevelPoints = 0;
                
                if (currentLevel && currentLevel.max !== null) {
                    const levelProgress = stats.reputation_points - currentLevel.min;
                    const levelRange = currentLevel.max - currentLevel.min;
                    progressPercent = Math.round((levelProgress / levelRange) * 100);
                    nextLevelPoints = currentLevel.max - stats.reputation_points;
                } else if (currentLevel && currentLevel.max === null) {
                    progressPercent = 100; // Max level
                }
                
                container.innerHTML = `
                    <div class="progress-header">
                        <span class="user-level" style="color: ${currentLevel.color};">${currentLevel.name}</span>
                        <span class="user-reputation">${stats.reputation_points} pts</span>
                    </div>
                    <div class="progress-bar">
                        <div class="progress-fill" style="width: ${progressPercent}%; background-color: ${currentLevel.color};"></div>
                    </div>
                    ${nextLevelPoints > 0 ? `<div style="font-size: 11px; color: #6b7280; text-align: center;">
                        ${nextLevelPoints} points to next level
                    </div>` : ''}
                    <div class="progress-stats">
                        <div class="progress-stat">
                            <div class="progress-stat-value">${stats.total_contributions}</div>
                            <div class="progress-stat-label">Contributions</div>
                        </div>
                        <div class="progress-stat">
                            <div class="progress-stat-value">${badges.length}</div>
                            <div class="progress-stat-label">Badges</div>
                        </div>
                    </div>
                `;
            } else {
                container.innerHTML = '<div class="no-data">No data available</div>';
            }
        } catch (error) {
            console.error('Error loading user progress:', error);
            container.innerHTML = '<div class="error-state">Failed to load progress</div>';
        }
    }
    
    // Update community stats
    function updateCommunityStats() {
        // You can implement this to fetch real community stats
        // For now, we'll use some dynamic values
        const stats = {
            members: '1.9k',
            contributions: '15.2k',
            badges: '540'
        };
        
        document.getElementById('total-members').textContent = stats.members;
        document.getElementById('total-contributions').textContent = stats.contributions;
        document.getElementById('badges-awarded').textContent = stats.badges;
    }
    
    // Auto-refresh community data every 5 minutes
    setInterval(() => {
        if (communityContent && communityContent.classList.contains('expanded')) {
            loadCommunityData();
        }
    }, 5 * 60 * 1000);
    
    // Handle clicks on leaderboard users
    document.addEventListener('click', function(e) {
        const userElement = e.target.closest('.leaderboard-user');
        if (userElement) {
            const userId = userElement.dataset.userId;
            // You can implement user profile popup or navigation here
            console.log('Clicked on user:', userId);
        }
    });
    
    // Handle badge hover effects
    document.addEventListener('mouseenter', function(e) {
        const badgeElement = e.target.closest('.badge-award');
        if (badgeElement) {
            // You can show badge details tooltip here
        }
    }, true);
});