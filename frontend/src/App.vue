<template>
    <div class="simple-shell">
        <header class="simple-header">
            <RouterLink class="simple-brand" to="/assignments">Homework Canvas</RouterLink>
            <nav class="simple-nav" aria-label="主导航">
                <RouterLink to="/assignments" active-class="active">DDL</RouterLink>
                <RouterLink to="/upload" active-class="active">上传</RouterLink>
            </nav>
        </header>
        <main class="simple-content"><RouterView /></main>
    </div>
</template>
