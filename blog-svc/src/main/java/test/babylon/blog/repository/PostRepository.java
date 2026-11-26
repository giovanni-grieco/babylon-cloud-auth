package test.babylon.blog.repository;

import org.springframework.data.jpa.repository.JpaRepository;
import test.babylon.blog.core.domain.Post;

public interface PostRepository extends JpaRepository<Post, Long>{
    
}
