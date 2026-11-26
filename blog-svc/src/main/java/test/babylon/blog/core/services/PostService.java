package test.babylon.blog.core.services;

import java.util.List;
import java.util.NoSuchElementException;

import org.springframework.stereotype.Service;

import lombok.AllArgsConstructor;
import lombok.Getter;
import lombok.Setter;
import test.babylon.blog.configs.AppProperties;
import test.babylon.blog.core.domain.Post;
import test.babylon.blog.repository.PostRepository;

@Service 
@AllArgsConstructor 
@Getter 
@Setter 
public class PostService {
    

    private final AppProperties appProperties;
    private final PostRepository postRepository;

    public List<Post> GetAllPosts(){
        return postRepository.findAll();
    }

    public Post GetPost(Long postId) throws NoSuchElementException{
        return postRepository.findById(postId).orElseThrow();
    }

    public Post NewPost(String title, String body, String author, String authorHomeRealm){
        
        if(!authorHomeRealm.equals(appProperties.realm())){
            throw new RuntimeException("Not Authorized");
        }

        Post post = new Post();
        post.setTitle(title);
        post.setBody(body);
        post.setAuthor(author);
        post.setAccessList(List.of());

        return postRepository.save(post);
    }
}
