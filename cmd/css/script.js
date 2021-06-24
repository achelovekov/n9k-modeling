const handleHover = function(e, opacity) {
    const link = e.target.closest('.menuItem')
    const siblings = link.closest('.mainBody').querySelectorAll('.menuItem')
    
    siblings.forEach(s => {
        if (s != link) s.style.opacity = opacity
    })

}
const body = document.querySelector('.mainBody')
body.addEventListener('mouseover', function(e) {
    handleHover(e, 0.5)
})
body.addEventListener('mouseout', function(e) {
    handleHover(e, 1.0)
})